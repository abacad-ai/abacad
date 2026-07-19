package api

import (
	"net/http"
	"strconv"

	"abacad/internal/store"
)

// record writes one row to the account trail. Control-plane actions all
// originate from the dashboard session, so the source is stamped here.
func (a *API) record(accountID string, act store.Activity) {
	act.AccountID = accountID
	if act.Source == "" {
		act.Source = "dashboard"
	}
	a.Activity.Record(act)
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
}

// listActivities returns the account's trail, newest first, with keyset
// pagination (?before=<id>) and optional ?device=, ?kind= (category prefix or
// exact), ?source= filters. next_before is absent once the trail is exhausted.
func (a *API) listActivities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ActivityFilter{
		DeviceID: q.Get("device"),
		Kind:     q.Get("kind"),
		Source:   q.Get("source"),
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
		})
	}
	resp := map[string]any{"activities": out}
	if len(acts) == f.Limit {
		resp["next_before"] = acts[len(acts)-1].ID
	}
	writeJSON(w, http.StatusOK, resp)
}
