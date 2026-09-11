package replay

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"abacad/internal/protocol"
	"abacad/internal/store"
)

func fixture(t *testing.T) (*store.Store, *Frames) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	fr, err := OpenFrames(t.TempDir() + "/frames")
	if err != nil {
		t.Fatalf("open frames: %v", err)
	}
	return st, fr
}

// screenshotReply builds a device reply carrying one frame, the way a real
// screenshot comes back over the wire.
func screenshotReply(t *testing.T, jpeg []byte, w, h int) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(protocol.ScreenshotResult{
		W: w, H: h, PNGBase64: base64.StdEncoding.EncodeToString(jpeg),
		Tree: &protocol.UITree{Pkg: "com.example", Nodes: []protocol.UITreeNode{{}}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// The core loop: a screenshot lands as a row plus a file on disk; a tap lands as
// a row with a marker and no file.
func TestWriteRecordsFramesAndFramelessSteps(t *testing.T) {
	st, fr := fixture(t)
	r := New(st, fr, 0, 0)

	r.write(Command{
		AccountID: "acc1", DeviceID: "dev1", Ts: 1000, Method: "screenshot",
		Source: "agent", Outcome: "ok", DurationMs: 12, ActorLabel: "laptop agent",
		Result: screenshotReply(t, []byte("jpegbytes"), 1080, 2400),
	})
	r.write(Command{
		AccountID: "acc1", DeviceID: "dev1", Ts: 2000, Method: "tap",
		Source: "agent", Outcome: "ok",
		Params: map[string]any{"x": 540, "y": 1180, "humanize": false},
	})

	steps, err := st.ReplaySteps("dev1", 0, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(steps))
	}

	shot := steps[0]
	if shot.FrameID == "" || shot.W != 1080 || shot.H != 2400 {
		t.Fatalf("screenshot step: %+v", shot)
	}
	if shot.ActorLabel != "laptop agent" || shot.DurationMs != 12 {
		t.Fatalf("metadata lost: %+v", shot)
	}
	f, _, err := fr.Open(shot.FrameID)
	if err != nil {
		t.Fatalf("frame file: %v", err)
	}
	got, _ := os.ReadFile(f.Name())
	f.Close()
	if string(got) != "jpegbytes" {
		t.Fatalf("frame bytes = %q", got)
	}

	tap := steps[1]
	if tap.FrameID != "" {
		t.Fatalf("a tap must not carry a frame: %+v", tap)
	}
	if tap.Marker != `{"kind":"tap","x":540,"y":1180}` {
		t.Fatalf("marker = %q", tap.Marker)
	}
}

// A composite runs several steps and returns one frame per screenshot among
// them; each becomes its own row so the timeline shows what the sequence saw.
func TestWriteRecordsEveryCompositeShot(t *testing.T) {
	st, fr := fixture(t)
	r := New(st, fr, 0, 0)

	res, err := json.Marshal(protocol.CompositeResult{Shots: []protocol.ScreenshotResult{
		{W: 10, H: 20, PNGBase64: base64.StdEncoding.EncodeToString([]byte("one"))},
		{W: 30, H: 40, PNGBase64: base64.StdEncoding.EncodeToString([]byte("two"))},
	}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	r.write(Command{AccountID: "acc1", DeviceID: "dev1", Ts: 1, Method: "composite", Outcome: "ok", Result: res})

	steps, _ := st.ReplaySteps("dev1", 0, 0, 0)
	if len(steps) != 2 {
		t.Fatalf("want one step per shot, got %d", len(steps))
	}
	if steps[0].W != 10 || steps[1].W != 30 {
		t.Fatalf("shots out of order or malformed: %+v", steps)
	}
}

// A failed command still belongs on the timeline — "the agent tried to tap and
// the device was gone" is exactly what someone replaying a session is looking
// for — but it has no frame to keep.
func TestWriteKeepsFailedCommandsWithoutFrames(t *testing.T) {
	st, fr := fixture(t)
	r := New(st, fr, 0, 0)

	r.write(Command{
		AccountID: "acc1", DeviceID: "dev1", Ts: 1, Method: "screenshot",
		Outcome: "timeout", Result: screenshotReply(t, []byte("stale"), 1, 1),
	})
	steps, _ := st.ReplaySteps("dev1", 0, 0, 0)
	if len(steps) != 1 {
		t.Fatalf("want the failed command recorded, got %d steps", len(steps))
	}
	if steps[0].FrameID != "" {
		t.Fatal("a non-ok command must not produce a frame")
	}
	entries, _ := os.ReadDir(fr.Dir())
	if len(entries) != 0 {
		t.Fatalf("wrote %d file(s) for a failed command", len(entries))
	}
}

// The dashboard's live view captures a frame every two seconds for as long as a
// device page is open. Recording those would bury the agent's own frames and
// blow through the per-device cap in minutes.
func TestRecordSkipsDashboardCommands(t *testing.T) {
	st, fr := fixture(t)
	r := New(st, fr, 0, 0)

	r.Record(Command{AccountID: "acc1", DeviceID: "dev1", Method: "screenshot", Source: "dashboard", Outcome: "ok",
		Result: screenshotReply(t, []byte("jpeg"), 1, 1)})
	r.Record(Command{AccountID: "acc1", DeviceID: "dev1", Method: "screenshot", Source: "agent", Outcome: "ok",
		Result: screenshotReply(t, []byte("jpeg"), 1, 1)})

	steps := waitForSteps(t, st, "dev1", 1)
	if steps[0].Source != "agent" {
		t.Fatalf("recorded the wrong command: %+v", steps[0])
	}
}

// A full buffer must cost a step, never a stall: the recorder sits on the
// relay's command path and a slow disk must not become a slow agent.
func TestRecordDropsInsteadOfBlocking(t *testing.T) {
	st, fr := fixture(t)
	// No write loop — nothing drains ch, so the buffer fills and stays full.
	r := &Recorder{st: st, frames: fr, ch: make(chan Command, 2)}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			r.Record(Command{AccountID: "acc1", DeviceID: "dev1", Method: "tap", Source: "agent"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Record blocked on a full buffer")
	}
	if r.Dropped() != 48 {
		t.Fatalf("dropped = %d, want 48", r.Dropped())
	}
}

// Forget is what device deletion calls. Frames are pictures of the device's
// screen; deleting the device must not leave them on disk.
func TestForgetRemovesRowsAndFiles(t *testing.T) {
	st, fr := fixture(t)
	r := New(st, fr, 0, 0)
	r.write(Command{AccountID: "acc1", DeviceID: "dev1", Ts: 1, Method: "screenshot", Outcome: "ok",
		Result: screenshotReply(t, []byte("jpeg"), 1, 1)})
	r.write(Command{AccountID: "acc1", DeviceID: "dev2", Ts: 1, Method: "screenshot", Outcome: "ok",
		Result: screenshotReply(t, []byte("jpeg"), 1, 1)})

	r.Forget("dev1")

	if got, _ := st.ReplaySteps("dev1", 0, 0, 0); len(got) != 0 {
		t.Fatalf("dev1 still has %d rows", len(got))
	}
	entries, _ := os.ReadDir(fr.Dir())
	if len(entries) != 1 {
		t.Fatalf("want dev2's frame left on disk, found %d file(s)", len(entries))
	}
}

// Rows are deleted before their files, so a crash in between strands bytes with
// no row left to find them by. The orphan sweep is what keeps the retention
// window a promise about the DISK rather than about the database.
func TestSweepOrphansRemovesFilesPastTheWindow(t *testing.T) {
	_, fr := fixture(t)
	if err := fr.Save("frm_old", []byte("jpeg")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := fr.Save("frm_new", []byte("jpeg")); err != nil {
		t.Fatalf("save: %v", err)
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(filepath.Join(fr.Dir(), "frm_old.jpg"), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if n := fr.sweepOrphans(time.Hour); n != 1 {
		t.Fatalf("swept %d file(s), want 1", n)
	}
	if _, _, err := fr.Open("frm_old"); err == nil {
		t.Fatal("the stale frame survived the sweep")
	}
	if _, _, err := fr.Open("frm_new"); err != nil {
		t.Fatalf("the fresh frame was swept: %v", err)
	}
	// A disabled window sweeps nothing, matching "0 = keep forever".
	if n := fr.sweepOrphans(0); n != 0 {
		t.Fatalf("a disabled window swept %d file(s)", n)
	}
}

// marker's allowlist is a security boundary, not a formatting choice: Params
// also carries typed text and evaluated code, and a denylist would leak the next
// verb somebody adds.
func TestMarkerTakesCoordinatesAndNothingElse(t *testing.T) {
	cases := []struct {
		method string
		params map[string]any
		want   string
	}{
		{"tap", map[string]any{"x": 1, "y": 2}, `{"kind":"tap","x":1,"y":2}`},
		{"click", map[string]any{"x": 1, "y": 2, "modifiers": []string{"cmd"}}, `{"kind":"click","x":1,"y":2}`},
		{"swipe", map[string]any{"x1": 1, "y1": 2, "x2": 3, "y2": 4}, `{"kind":"swipe","x":1,"y":2,"x2":3,"y2":4}`},
		{"scroll", map[string]any{"x": 1, "y": 2, "dy": 9}, `{"kind":"scroll","x":1,"y":2}`},
		// Nothing positional, or nothing safe to keep.
		{"input_text", map[string]any{"text": "hunter2"}, ""},
		{"execute", map[string]any{"code": "fetch('/x')"}, ""},
		{"screenshot", map[string]any{"include_ui_tree": true}, ""},
		{"back", nil, ""},
		// A pointer verb whose coordinates aren't numbers yields nothing rather
		// than a marker pointing at (0,0).
		{"tap", map[string]any{"x": "540", "y": 1180}, ""},
	}
	for _, c := range cases {
		if got := marker(c.method, c.params); got != c.want {
			t.Errorf("marker(%s) = %q, want %q", c.method, got, c.want)
		}
	}
}

// The UI tree rides along in a screenshot reply and is often larger than the
// JPEG. It is the agent's working data and adds nothing to a picture of the
// screen it describes, so it must not reach the recorder's output.
func TestFramesOfDropsTheUITree(t *testing.T) {
	c := Command{Method: "screenshot", Outcome: "ok", Result: screenshotReply(t, []byte("jpeg"), 1, 1)}
	shots := framesOf(c)
	if len(shots) != 1 {
		t.Fatalf("want 1 shot, got %d", len(shots))
	}
	if shots[0].Tree != nil {
		t.Fatal("the UI tree survived into the recorded frame")
	}
}

func waitForSteps(t *testing.T, st *store.Store, deviceID string, want int) []store.ReplayStep {
	t.Helper()
	for i := 0; i < 100; i++ {
		steps, err := st.ReplaySteps(deviceID, 0, 0, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(steps) >= want {
			// Give a wrongly-recorded extra step a chance to show up too.
			time.Sleep(20 * time.Millisecond)
			steps, _ = st.ReplaySteps(deviceID, 0, 0, 0)
			if len(steps) != want {
				t.Fatalf("want %d steps, got %d", want, len(steps))
			}
			return steps
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d step(s)", want)
	return nil
}
