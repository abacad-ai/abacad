package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"abacad/internal/store"
)

// Session replay endpoints: the recorded picture track behind the dashboard's
// Replay card. Three routes, in the order a player uses them:
//
//	GET /api/devices/{id}/replay/sessions       what sessions exist
//	GET /api/devices/{id}/replay?from=&to=      the steps of one
//	GET /api/devices/{id}/replay/frames/{frame} one frame's bytes
//
// All three are session-authenticated and re-check device ownership, exactly
// like deviceScreenshot. Recording is per-device and off by default; see
// docs/replay.md.

// sessionGap is how long a device may go without a recorded command before the
// next one starts a new session.
//
// There is no session id on the wire — an agent does not announce that it has
// started or finished a task, and inventing an id would mean asserting a
// boundary nobody told us about. A gap is the honest approximation: three
// minutes is far longer than the pause between two steps of one task (seconds)
// and far shorter than the pause between two tasks.
const sessionGapMs int64 = 3 * 60 * 1000

// maxSessionScan bounds how many marks the session index reads. At the default
// per-device cap this covers every retained step; a device configured well above
// it gets its most recent sessions, which is what the picker shows anyway.
const maxSessionScan = 5000

// replayStepView is one step of a session. FrameURL is absent for a step that
// captured no image — the player draws Marker over the frame before it.
type replayStepView struct {
	ID         int64  `json:"id"`
	Ts         int64  `json:"ts"` // unix millis, matching the events endpoint
	Method     string `json:"method"`
	Source     string `json:"source,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	ActorLabel string `json:"actor_label,omitempty"`
	FrameURL   string `json:"frame_url,omitempty"`
	W          int    `json:"w,omitempty"`
	H          int    `json:"h,omitempty"`
	// Marker is the pointer coordinates this step acted on, already JSON —
	// {"kind":"tap","x":540,"y":1180} — or absent. Passed through rather than
	// re-parsed: the recorder is the only writer and its shape is the contract.
	Marker json.RawMessage `json:"marker,omitempty"`
}

// replaySessionView is one contiguous run of recorded activity.
type replaySessionView struct {
	StartTs    int64  `json:"start_ts"`
	EndTs      int64  `json:"end_ts"`
	Steps      int    `json:"steps"`
	Frames     int    `json:"frames"`
	ActorLabel string `json:"actor_label,omitempty"`
}

// replayDevice resolves the device in the path and confirms the caller owns it,
// writing the error response itself. The bool reports whether to continue.
func (a *API) replayDevice(w http.ResponseWriter, r *http.Request) (store.Device, bool) {
	d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return store.Device{}, false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load device")
		return store.Device{}, false
	}
	return d, true
}

// replaySessions lists a device's recorded sessions, newest first, splitting its
// history wherever the gap between consecutive steps exceeds sessionGapMs.
//
// The grouping is done here rather than in SQL because it is a scan over an
// ordered column with one comparison per row — the kind of thing Go does in a
// few microseconds and a window function makes unreadable.
func (a *API) replaySessions(w http.ResponseWriter, r *http.Request) {
	d, ok := a.replayDevice(w, r)
	if !ok {
		return
	}
	marks, err := a.Store.ReplayMarks(d.ID, maxSessionScan)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list replay sessions")
		return
	}

	sessions := make([]replaySessionView, 0, 8)
	for i, m := range marks {
		if i == 0 || m.Ts-marks[i-1].Ts > sessionGapMs {
			sessions = append(sessions, replaySessionView{StartTs: m.Ts, ActorLabel: m.ActorLabel})
		}
		s := &sessions[len(sessions)-1]
		s.EndTs = m.Ts
		s.Steps++
		if m.HasFrame {
			s.Frames++
		}
		// The actor can change mid-session (a person takes over from an agent).
		// Keep the first one seen: it is who started the session, which is the
		// label the picker wants.
		if s.ActorLabel == "" {
			s.ActorLabel = m.ActorLabel
		}
	}
	// Newest first, matching every other listing in the dashboard.
	for i, j := 0, len(sessions)-1; i < j; i, j = i+1, j-1 {
		sessions[i], sessions[j] = sessions[j], sessions[i]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		// Whether recording is on right now, so an empty list can say WHY it is
		// empty — "recording is off" and "nothing recorded yet" look identical
		// otherwise, and only one of them is something the user should act on.
		"recording": d.Replay,
	})
}

// replaySteps returns one session's steps, oldest first: ?from= and ?to= are the
// session's bounds in unix millis, as returned by replaySessions.
func (a *API) replaySteps(w http.ResponseWriter, r *http.Request) {
	d, ok := a.replayDevice(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
	to, _ := strconv.ParseInt(q.Get("to"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))

	steps, err := a.Store.ReplaySteps(d.ID, from, to, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load replay")
		return
	}
	out := make([]replayStepView, 0, len(steps))
	for _, s := range steps {
		v := replayStepView{
			ID: s.ID, Ts: s.Ts, Method: s.Method, Source: s.Source, Outcome: s.Outcome,
			DurationMs: s.DurationMs, ActorLabel: s.ActorLabel, W: s.W, H: s.H,
		}
		if s.FrameID != "" {
			v.FrameURL = "/api/devices/" + d.ID + "/replay/frames/" + s.FrameID
		}
		if s.Marker != "" {
			v.Marker = json.RawMessage(s.Marker)
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": out, "recording": d.Replay})
}

// replayFrame serves one recorded frame as a JPEG.
//
// Authorization does not trust the id in the path: the frame's own row names the
// device and account that own it, and both must match the caller and the device
// in the path. A mismatch is a 404, never a 403 — an id's existence must not leak
// across accounts.
func (a *API) replayFrame(w http.ResponseWriter, r *http.Request) {
	d, ok := a.replayDevice(w, r)
	if !ok {
		return
	}
	if a.ReplayFrames == nil {
		writeErr(w, http.StatusNotFound, "frame not found")
		return
	}
	id := r.PathValue("frame")
	deviceID, accountID, err := a.Store.ReplayFrame(id)
	if err != nil || deviceID != d.ID || accountID != account(r).ID {
		writeErr(w, http.StatusNotFound, "frame not found")
		return
	}
	f, modTime, err := a.ReplayFrames.Open(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "frame not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	// Unlike the live screenshot (no-store, because the URL's content changes
	// under it), a frame id names one immutable image. Caching it privately is
	// what makes scrubbing back and forth over a session smooth.
	w.Header().Set("Cache-Control", "private, max-age=3600, immutable")
	http.ServeContent(w, r, "", modTime, f)
}
