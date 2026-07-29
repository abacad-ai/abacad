package api

import (
	"net/http"
	"strconv"

	"abacad/internal/auth"
	"abacad/internal/relay"
	"abacad/internal/store"
)

// record writes one row to the account trail. Control-plane actions all
// originate from the dashboard session, so the source is stamped here — as is the
// provenance the request carries: which session, from which address.
//
// The actor is only defaulted, never overwritten, so a caller that knows better
// (an action taken on behalf of a CLI pairing, say) can set its own and keep it.
func (a *API) record(r *http.Request, accountID string, act store.Activity) {
	act.AccountID = accountID
	if act.Source == "" {
		act.Source = "dashboard"
	}
	// Only claim a session when the request actually carries one. Events that
	// happen without one — a failed sign-in, a CLI polling its pairing — are left
	// with a blank actor on purpose: the party is unknown, and naming the account
	// owner as the actor of a failed password attempt would assert the opposite of
	// what happened. IP and User-Agent below are what those rows have to offer.
	if act.ActorKind == "" {
		if sid := auth.SessionID(r); sid != "" {
			act.ActorKind = store.ActorSession
			act.ActorID = auth.SessionFingerprint(sid)
			if act.ActorLabel == "" {
				act.ActorLabel = account(r).Email
			}
		}
	}
	act.IP = auth.ClientIP(r)
	act.UserAgent = r.UserAgent()
	a.Activity.Record(act)
}

// sessionActor describes the signed-in caller for commands relayed on their
// behalf. Those are recorded far from this request — on the device's goroutine —
// so the identity travels down on the context (relay.WithActor).
func sessionActor(r *http.Request) relay.Actor {
	return relay.Actor{
		Kind:      store.ActorSession,
		ID:        auth.SessionFingerprint(auth.SessionID(r)),
		Label:     account(r).Email,
		IP:        auth.ClientIP(r),
		UserAgent: r.UserAgent(),
	}
}

// activityView is one row of GET /api/activities. Ts is unix millis, matching
// the device events endpoint so the frontend shares its time helpers.
type activityView struct {
	ID         int64  `json:"id"`
	Ts         int64  `json:"ts"`
	Kind       string `json:"kind"`
	DeviceID   string `json:"device_id,omitempty"`
	Method     string `json:"method,omitempty"`
	Source     string `json:"source,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Detail     string `json:"detail,omitempty"`

	// Provenance. All omitempty: rows written before this existed have none, and
	// some events legitimately have no actor (see record). ActorID is safe to
	// expose — it is a key id or a session *fingerprint*, never a live credential.
	ActorKind  string `json:"actor_kind,omitempty"`
	ActorID    string `json:"actor_id,omitempty"`
	ActorLabel string `json:"actor_label,omitempty"`
	IP         string `json:"ip,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`

	// Where the IP resolved to when the row was written, if the relay has a geo
	// database. Country is an ISO 3166-1 alpha-2 code; City is frequently absent
	// even when Country is known, and is only ever approximate.
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
}

// listActivities returns the account's trail, newest first, with keyset
// pagination (?before=<id>) and optional ?device=, ?kind= (category prefix or
// exact), ?source=, ?actor= (exact credential id), ?ip= and ?country= (ISO
// alpha-2) filters. next_before is absent once the trail is exhausted.
func (a *API) listActivities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ActivityFilter{
		DeviceID: q.Get("device"),
		Kind:     q.Get("kind"),
		Source:   q.Get("source"),
		ActorID:  q.Get("actor"),
		IP:       q.Get("ip"),
		Country:  q.Get("country"),
	}
	if v := q.Get("before"); v != "" {
		f.BeforeID, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("limit"); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}

	acts, err := a.Store.Activities(account(r).ID, f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list activities")
		return
	}
	out := make([]activityView, 0, len(acts))
	for _, act := range acts {
		out = append(out, activityView{
			ID: act.ID, Ts: act.Ts, Kind: act.Kind, DeviceID: act.DeviceID,
			Method: act.Method, Source: act.Source, Outcome: act.Outcome,
			DurationMs: act.DurationMs, Detail: act.Detail,
			ActorKind: act.ActorKind, ActorID: act.ActorID, ActorLabel: act.ActorLabel,
			IP: act.IP, UserAgent: act.UserAgent, Country: act.Country, City: act.City,
		})
	}
	resp := map[string]any{"activities": out}
	if len(acts) == f.Limit {
		resp["next_before"] = acts[len(acts)-1].ID
	}
	writeJSON(w, http.StatusOK, resp)
}
