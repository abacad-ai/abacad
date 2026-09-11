package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"abacad/internal/replay"
	"abacad/internal/store"
)

// replayFixture is gateFixture plus a frame store, one account with a device,
// and a second account that must never see any of it.
func replayFixture(t *testing.T) (*API, store.Account, store.Device, store.Account) {
	t.Helper()
	a, acc := gateFixture(t)
	frames, err := replay.OpenFrames(t.TempDir() + "/frames")
	if err != nil {
		t.Fatalf("frames: %v", err)
	}
	a.ReplayFrames = frames

	dev, _, err := a.Store.CreateDevice(acc.ID, "phone", "android", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	other, err := a.Store.CreateAccount("b@x.test", "hash")
	if err != nil {
		t.Fatalf("other account: %v", err)
	}
	return a, acc, dev, other
}

// serve runs one replay handler with the path values and session the router
// would have set.
func serve(h http.HandlerFunc, as store.Account, deviceID string, query string, pathValues map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/api/devices/"+deviceID+"/replay"+query, nil)
	r.SetPathValue("id", deviceID)
	for k, v := range pathValues {
		r.SetPathValue(k, v)
	}
	r = r.WithContext(context.WithValue(r.Context(), accountKey, as))
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestReplaySessionsSplitsOnGap(t *testing.T) {
	a, acc, dev, _ := replayFixture(t)
	// Two bursts about sixteen minutes apart — well past sessionGapMs — with a
	// frameless tap in the middle of the first.
	rows := []store.ReplayStep{
		{Ts: 1_000_000, Method: "screenshot", FrameID: "frm_a"},
		{Ts: 1_000_500, Method: "tap"},
		{Ts: 1_030_000, Method: "screenshot", FrameID: "frm_b"},
		{Ts: 2_000_000, Method: "screenshot", FrameID: "frm_c"},
		{Ts: 2_000_800, Method: "tap"},
	}
	for _, row := range rows {
		row.AccountID, row.DeviceID, row.Outcome, row.ActorLabel = acc.ID, dev.ID, "ok", "laptop agent"
		if err := a.Store.InsertReplayStep(row); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	w := serve(a.replaySessions, acc, dev.ID, "/sessions", nil)
	if w.Code != 200 {
		t.Fatalf("code = %d, body %s", w.Code, w.Body)
	}
	var got struct {
		Sessions  []replaySessionView `json:"sessions"`
		Recording bool                `json:"recording"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d (%+v)", len(got.Sessions), got.Sessions)
	}
	// Newest first, matching every other listing in the dashboard.
	if got.Sessions[0].StartTs != 2_000_000 || got.Sessions[0].Steps != 2 || got.Sessions[0].Frames != 1 {
		t.Fatalf("newest session: %+v", got.Sessions[0])
	}
	older := got.Sessions[1]
	if older.StartTs != 1_000_000 || older.EndTs != 1_030_000 || older.Steps != 3 || older.Frames != 2 {
		t.Fatalf("older session: %+v", older)
	}
	if older.ActorLabel != "laptop agent" {
		t.Fatalf("actor label = %q", older.ActorLabel)
	}
	// Recording is off for this device. The flag is what lets an empty list say
	// WHY it is empty — "off" and "nothing yet" are identical without it, and
	// only one of them is something the user should act on.
	if got.Recording {
		t.Fatal("recording reported on for a device that never enabled it")
	}
}

func TestReplayStepsRangeAndFrameURL(t *testing.T) {
	a, acc, dev, _ := replayFixture(t)
	rows := []store.ReplayStep{
		{Ts: 100, Method: "screenshot", FrameID: "frm_a", W: 10, H: 20},
		{Ts: 200, Method: "tap", Marker: `{"kind":"tap","x":5,"y":6}`},
		{Ts: 900, Method: "screenshot", FrameID: "frm_b"},
	}
	for _, row := range rows {
		row.AccountID, row.DeviceID, row.Outcome = acc.ID, dev.ID, "ok"
		if err := a.Store.InsertReplayStep(row); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	w := serve(a.replaySteps, acc, dev.ID, "?from=100&to=200", nil)
	if w.Code != 200 {
		t.Fatalf("code = %d, body %s", w.Code, w.Body)
	}
	var got struct {
		Steps []replayStepView `json:"steps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("want the 2 steps in range, got %d", len(got.Steps))
	}
	if got.Steps[0].Ts != 100 || got.Steps[1].Ts != 200 {
		t.Fatalf("want oldest first, got %d then %d", got.Steps[0].Ts, got.Steps[1].Ts)
	}
	want := "/api/devices/" + dev.ID + "/replay/frames/frm_a"
	if got.Steps[0].FrameURL != want {
		t.Fatalf("frame url = %q, want %q", got.Steps[0].FrameURL, want)
	}
	if got.Steps[0].W != 10 || got.Steps[0].H != 20 {
		t.Fatalf("frame size lost: %+v", got.Steps[0])
	}
	if got.Steps[1].FrameURL != "" {
		t.Fatal("a frameless step got a frame url")
	}
	if string(got.Steps[1].Marker) != `{"kind":"tap","x":5,"y":6}` {
		t.Fatalf("marker = %s", got.Steps[1].Marker)
	}
}

// A frame id is not a capability. Its row names the device and account that own
// it, and both must match the caller — a mismatch is 404, never 403, so an id's
// existence never leaks across accounts.
func TestReplayFrameIsScopedToItsOwner(t *testing.T) {
	a, acc, dev, other := replayFixture(t)
	if err := a.ReplayFrames.Save("frm_a", []byte("jpegbytes")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := a.Store.InsertReplayStep(store.ReplayStep{
		AccountID: acc.ID, DeviceID: dev.ID, Ts: 1, Method: "screenshot", Outcome: "ok", FrameID: "frm_a",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	sibling, _, err := a.Store.CreateDevice(acc.ID, "laptop", "macos", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}

	fetch := func(as store.Account, deviceID, frame string) int {
		return serve(a.replayFrame, as, deviceID, "/frames/"+frame, map[string]string{"frame": frame}).Code
	}

	if code := fetch(acc, dev.ID, "frm_a"); code != 200 {
		t.Fatalf("owner fetch = %d, want 200", code)
	}
	// Another account holding the exact frame id is stopped at the device gate.
	if code := fetch(other, dev.ID, "frm_a"); code != 404 {
		t.Fatalf("cross-account fetch = %d, want 404", code)
	}
	// The right account, but asking through a device that doesn't own the frame.
	if code := fetch(acc, sibling.ID, "frm_a"); code != 404 {
		t.Fatalf("wrong-device fetch = %d, want 404", code)
	}
	if code := fetch(acc, dev.ID, "frm_missing"); code != 404 {
		t.Fatalf("unknown frame = %d, want 404", code)
	}
}

// The listings are gated on device ownership before a single step is read, the
// same shape deviceScreenshot uses.
func TestReplayListingsAreAccountScoped(t *testing.T) {
	a, acc, dev, other := replayFixture(t)
	if err := a.Store.InsertReplayStep(store.ReplayStep{
		AccountID: acc.ID, DeviceID: dev.ID, Ts: 1, Method: "screenshot", Outcome: "ok", FrameID: "frm_a",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if code := serve(a.replaySessions, other, dev.ID, "/sessions", nil).Code; code != 404 {
		t.Fatalf("cross-account sessions = %d, want 404", code)
	}
	if code := serve(a.replaySteps, other, dev.ID, "", nil).Code; code != 404 {
		t.Fatalf("cross-account steps = %d, want 404", code)
	}
}

// TestReplayAttestationGate: enabling recording requires attested=true, and both
// directions land on the trail — when recording started and when it stopped are
// what make the recording itself accountable.
func TestReplayAttestationGate(t *testing.T) {
	a, acc, dev, _ := replayFixture(t)

	call := func(body string) int {
		r := httptest.NewRequest("PATCH", "/api/devices/"+dev.ID, strings.NewReader(body))
		r.SetPathValue("id", dev.ID)
		r = r.WithContext(context.WithValue(r.Context(), accountKey, acc))
		w := httptest.NewRecorder()
		a.updateDevice(w, r)
		return w.Code
	}

	if code := call(`{"replay":true}`); code != 422 {
		t.Fatalf("enable without attestation: got %d, want 422", code)
	}
	if d, _ := a.Store.DeviceByID(dev.ID); d.Replay {
		t.Fatal("recording turned on despite the missing attestation")
	}
	if code := call(`{"replay":true,"attested":true}`); code != 204 {
		t.Fatalf("enable with attestation: got %d, want 204", code)
	}
	if d, _ := a.Store.DeviceByID(dev.ID); !d.Replay {
		t.Fatal("replay not persisted after attested enable")
	}
	if !hasConsent(t, a.Store, acc.ID, "replay.enable") {
		t.Fatal("no consent activity recorded for replay.enable")
	}

	if code := call(`{"replay":false}`); code != 204 {
		t.Fatalf("disable: got %d, want 204", code)
	}
	if d, _ := a.Store.DeviceByID(dev.ID); d.Replay {
		t.Fatal("replay still on after disable")
	}
	if !hasConsent(t, a.Store, acc.ID, "replay.disable") {
		t.Fatal("turning recording off left no trace on the trail")
	}
}
