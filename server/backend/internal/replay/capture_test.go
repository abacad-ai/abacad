package replay

import (
	"encoding/base64"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"abacad/internal/protocol"
	"abacad/internal/store"
)

// fakeCapturer stands in for the device. Each call returns the next queued frame
// (or repeats the last), and records that it was asked.
type fakeCapturer struct {
	mu     sync.Mutex
	frames []string // the jpeg bytes to hand back, in order
	err    error
	calls  int
}

func (f *fakeCapturer) Capture(string) (protocol.ScreenshotResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return protocol.ScreenshotResult{}, f.err
	}
	jpeg := "after"
	if len(f.frames) > 0 {
		jpeg = f.frames[0]
		if len(f.frames) > 1 {
			f.frames = f.frames[1:]
		}
	}
	return protocol.ScreenshotResult{W: 10, H: 20, PNGBase64: base64.StdEncoding.EncodeToString([]byte(jpeg))}, nil
}

func (f *fakeCapturer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// settle is short enough to keep tests fast, long enough that a "did it fire
// early?" assertion has room to be meaningful.
const testSettle = 60 * time.Millisecond

func recorderWithCapture(t *testing.T, cap Capturer) (*Recorder, *store.Store, *Frames) {
	t.Helper()
	st, fr := fixture(t)
	return New(st, fr, 0, 0, WithCapturer(cap, testSettle)), st, fr
}

func action(method string, ts int64) Command {
	return Command{AccountID: "acc1", DeviceID: "dev1", Ts: ts, Method: method, Source: "agent", Outcome: "ok"}
}

// The headline behaviour: a tap produces a picture of what it did, recorded as
// its own step and tagged as the server's own capture rather than the agent's.
func TestActionGetsAFollowUpFrame(t *testing.T) {
	cap := &fakeCapturer{}
	r, st, _ := recorderWithCapture(t, cap)

	r.write(action("tap", 1000))
	steps := waitForSteps(t, st, "dev1", 2)

	if steps[0].Method != "tap" || steps[0].FrameID != "" {
		t.Fatalf("the tap itself should stay frameless: %+v", steps[0])
	}
	after := steps[1]
	if after.Method != "screenshot" || after.Source != SourceReplay {
		t.Fatalf("follow-up: %+v", after)
	}
	if after.FrameID == "" || after.W != 10 || after.H != 20 {
		t.Fatalf("follow-up carried no frame: %+v", after)
	}
	if after.Ts < steps[0].Ts {
		t.Fatalf("follow-up is stamped before the action it follows: %d < %d", after.Ts, steps[0].Ts)
	}
}

// The allowlist. A verb that already carries pixels, or that changes no screen,
// must not cost the device an extra round trip.
func TestFollowUpOnlyAfterScreenChangingActions(t *testing.T) {
	for _, method := range []string{"screenshot", "composite", "push_file", "pull_file", "screen_recording"} {
		t.Run(method, func(t *testing.T) {
			cap := &fakeCapturer{}
			r, _, _ := recorderWithCapture(t, cap)
			r.write(action(method, 1000))
			time.Sleep(testSettle * 3)
			if n := cap.count(); n != 0 {
				t.Fatalf("%s triggered %d capture(s)", method, n)
			}
		})
	}
	for _, method := range []string{"tap", "click", "swipe", "input_text", "back", "press_keys", "execute"} {
		t.Run(method, func(t *testing.T) {
			cap := &fakeCapturer{}
			r, _, _ := recorderWithCapture(t, cap)
			r.write(action(method, 1000))
			waitFor(t, func() bool { return cap.count() == 1 }, "capture after "+method)
		})
	}
}

// A failed action changed nothing, so there is nothing new to photograph.
func TestNoFollowUpAfterAFailedAction(t *testing.T) {
	cap := &fakeCapturer{}
	r, _, _ := recorderWithCapture(t, cap)

	c := action("tap", 1000)
	c.Outcome = "timeout"
	r.write(c)
	c.Outcome = "denied"
	r.write(c)

	time.Sleep(testSettle * 3)
	if n := cap.count(); n != 0 {
		t.Fatalf("failed actions triggered %d capture(s)", n)
	}
}

// The coalesce rule, and the reason the feature is cheap in the common case: the
// agent's own screenshot arriving during the settle window IS the after-picture,
// so the follow-up must not also fire.
func TestAgentScreenshotDuringSettleCancelsTheFollowUp(t *testing.T) {
	cap := &fakeCapturer{}
	r, st, _ := recorderWithCapture(t, cap)

	r.write(action("tap", 1000))
	// The agent looks at the result itself, before our timer fires.
	shot := action("screenshot", 1100)
	shot.Result = screenshotReply(t, []byte("agent-frame"), 1, 1)
	r.write(shot)

	time.Sleep(testSettle * 4)
	if n := cap.count(); n != 0 {
		t.Fatalf("captured %d frame(s) despite the agent's own screenshot", n)
	}
	steps, _ := st.ReplaySteps("dev1", 0, 0, 0)
	if len(steps) != 2 {
		t.Fatalf("want just the tap and the agent's screenshot, got %d steps", len(steps))
	}
}

// The frame BEFORE an action must not cancel that action's follow-up, even when
// the two share a timestamp.
//
// This is a regression test with a story. The coalesce rule was first written as
// "skip if a frame exists with ts >= the action's ts", which is the right idea
// expressed in the wrong units: commands are stamped in unix milliseconds, and on
// a fast local device the agent's screenshot and the tap that follows it land in
// the same millisecond most of the time. The rule then read a frame taken BEFORE
// the tap as proof the tap's result had already been photographed, and silently
// dropped the picture. It reproduced roughly one run in two.
//
// The fix is to stop asking the question with clocks: a per-device frame counter
// snapshotted when the capture is scheduled answers "has a frame landed SINCE",
// which is what was meant, and cannot be defeated by two events sharing a tick.
func TestFrameInTheSameMillisecondDoesNotCancelTheFollowUp(t *testing.T) {
	cap := &fakeCapturer{}
	r, st, _ := recorderWithCapture(t, cap)

	const sameTs = 1000
	shot := action("screenshot", sameTs)
	shot.Result = screenshotReply(t, []byte("before"), 1, 1)
	r.write(shot)
	r.write(action("tap", sameTs)) // identical timestamp, deliberately

	waitFor(t, func() bool { return cap.count() == 1 }, "the follow-up despite the shared timestamp")
	steps := waitForSteps(t, st, "dev1", 3)
	if steps[2].Source != SourceReplay || steps[2].FrameID == "" {
		t.Fatalf("third step should be the captured after-frame: %+v", steps[2])
	}
}

// A burst of actions wants one picture of where the burst ended, not one per
// action: the pending capture resets rather than stacking.
func TestBurstOfActionsCapturesOnce(t *testing.T) {
	cap := &fakeCapturer{}
	r, st, _ := recorderWithCapture(t, cap)

	for i := 0; i < 5; i++ {
		r.write(action("tap", int64(1000+i)))
		time.Sleep(testSettle / 4)
	}
	waitFor(t, func() bool { return cap.count() == 1 }, "exactly one capture after a burst")
	time.Sleep(testSettle * 2)
	if n := cap.count(); n != 1 {
		t.Fatalf("burst produced %d captures, want 1", n)
	}
	steps, _ := st.ReplaySteps("dev1", 0, 0, 0)
	if len(steps) != 6 { // five taps + one frame
		t.Fatalf("want 5 taps + 1 frame, got %d steps", len(steps))
	}
}

// A follow-up that finds the screen exactly as the last frame left it has
// nothing to add, and is dropped whole — no row, no file.
func TestIdenticalFollowUpFrameIsDropped(t *testing.T) {
	cap := &fakeCapturer{frames: []string{"same"}}
	r, st, fr := recorderWithCapture(t, cap)

	// Seed the device's last frame with the very bytes the capturer will return.
	shot := action("screenshot", 1000)
	shot.Result = screenshotReply(t, []byte("same"), 1, 1)
	r.write(shot)
	waitForSteps(t, st, "dev1", 1)

	r.write(action("tap", 2000))
	waitFor(t, func() bool { return cap.count() == 1 }, "the follow-up to run")
	time.Sleep(testSettle)

	steps, _ := st.ReplaySteps("dev1", 0, 0, 0)
	if len(steps) != 2 { // the seed screenshot and the tap; no third row
		t.Fatalf("an unchanged screen was recorded anyway: %d steps", len(steps))
	}
	entries, _ := os.ReadDir(fr.Dir())
	if len(entries) != 1 {
		t.Fatalf("want only the seed frame on disk, found %d", len(entries))
	}
}

// An agent's own screenshot of an unchanged screen is still a command it really
// issued, and the timeline has to show it. Only OUR captures are deduped.
func TestIdenticalAgentFrameIsStillRecorded(t *testing.T) {
	r, st, _ := recorderWithCapture(t, &fakeCapturer{})

	for _, ts := range []int64{1000, 2000} {
		shot := action("screenshot", ts)
		shot.Result = screenshotReply(t, []byte("same"), 1, 1)
		r.write(shot)
	}
	steps := waitForSteps(t, st, "dev1", 2)
	if steps[0].FrameID == "" || steps[1].FrameID == "" {
		t.Fatalf("an agent screenshot lost its frame to dedup: %+v", steps)
	}
}

// The device went away, fell asleep, or stopped exposing screenshot between the
// action and the capture. That is an ordinary outcome: record nothing, retry
// nothing.
func TestCaptureErrorRecordsNothingAndDoesNotRetry(t *testing.T) {
	cap := &fakeCapturer{err: errors.New("skipped")}
	r, st, _ := recorderWithCapture(t, cap)

	r.write(action("tap", 1000))
	waitFor(t, func() bool { return cap.count() == 1 }, "the capture attempt")
	time.Sleep(testSettle * 4)

	if n := cap.count(); n != 1 {
		t.Fatalf("a failed capture was retried (%d calls)", n)
	}
	steps, _ := st.ReplaySteps("dev1", 0, 0, 0)
	if len(steps) != 1 || steps[0].Method != "tap" {
		t.Fatalf("a failed capture left a row: %+v", steps)
	}
}

// Without a capturer the recorder behaves exactly as it did before follow-ups
// existed — the operator's off switch has to be a real off switch.
func TestNoCapturerMeansNoFollowUps(t *testing.T) {
	st, fr := fixture(t)
	r := New(st, fr, 0, 0) // no WithCapturer
	r.write(action("tap", 1000))
	time.Sleep(testSettle * 3)
	steps, _ := st.ReplaySteps("dev1", 0, 0, 0)
	if len(steps) != 1 {
		t.Fatalf("want just the tap, got %d steps", len(steps))
	}
	_ = fr
}

// WithCapturer is the knob's implementation, so its own guards matter: a nil
// capturer or a non-positive settle must leave the feature off rather than
// panicking or firing instantly.
func TestWithCapturerRejectsUnusableSettings(t *testing.T) {
	st, fr := fixture(t)
	cap := &fakeCapturer{}
	for _, tc := range []struct {
		name   string
		c      Capturer
		settle time.Duration
	}{
		{"nil capturer", nil, testSettle},
		{"zero settle", cap, 0},
		{"negative settle", cap, -time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := New(st, fr, 0, 0, WithCapturer(tc.c, tc.settle))
			if r.capturer != nil {
				t.Fatal("capture enabled with an unusable setting")
			}
		})
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
