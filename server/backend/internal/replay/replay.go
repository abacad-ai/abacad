// Package replay records what an agent did on a device, frame by frame, so a
// human can watch it back afterwards.
//
// The activity trail already answers "what commands ran"; this answers "what did
// the screen look like while they ran". It is the audit surface's picture track:
// every relayed command becomes a step, and the ones that returned an image
// (only screenshot and composite do) carry a frame.
//
// Action verbs — tap, click, swipe — return no pixels, so an action is recorded
// as a frameless step, its marker drawn over the frame BEFORE it (the screen it
// acted on, and the only one its coordinates mean anything against). To also show
// what it produced, the recorder takes its own screenshot a moment later, unless
// one has already arrived. So a session reads:
//
//	[frame] --tap--> [frame after] --swipe--> [frame after] --> ...
//
// That follow-up is the one place in abacad where an observability feature sends
// a command to a device, which is why it is bounded on every side: only after a
// successful action, only on a device that is awake, online and exposing
// screenshot, only one in flight per device, and never when the agent's own next
// screenshot got there first.
//
// Two properties are load-bearing:
//
//   - It is OFF by default, per device, and turning it on takes a deliberate
//     human act in the dashboard. Keeping every frame an agent sees is screen
//     content at rest; it should never start happening because a device was
//     enrolled.
//   - It is never on the command's critical path. Record hands the payload to a
//     buffered channel and returns; a single goroutine decodes, writes the JPEG
//     and inserts the row. A full buffer drops the step rather than stalling the
//     relay, exactly as activity.Recorder does — replay is an observability
//     surface, and a slow disk must not become a slow agent.
package replay

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"abacad/internal/auth"
	"abacad/internal/protocol"
	"abacad/internal/store"
)

// dropReportInterval bounds how often a full buffer is reported, coalescing a
// burst of drops into one log line. Matches activity.dropReportInterval.
const dropReportInterval = time.Minute

// gcInterval is how often the retention sweep runs. Retention is measured in
// hours, so a quarter-hour sweep bounds over-retention to a rounding error.
const gcInterval = 15 * time.Minute

// SourceReplay tags the screenshots the recorder takes on its own behalf, after
// an action, so they are distinguishable everywhere a source is: the activity
// trail, its filters, and the player. They are real commands the device really
// served — hiding them would leave the server's record disagreeing with the
// device's own, and would let a supervision feature quietly inflate a device's
// command count.
const SourceReplay = "replay"

// actionVerbs are the commands that change what is on screen, and therefore the
// ones worth a follow-up frame. The list is an allowlist for the same reason
// marker's is: a denylist would silently opt in the next verb somebody adds.
//
// Absent on purpose: screenshot and composite already carry their own pixels,
// and push_file/pull_file/screen_recording/vnc change no screen.
var actionVerbs = map[protocol.Method]bool{
	protocol.MethodTap:        true,
	protocol.MethodLongPress:  true,
	protocol.MethodSwipe:      true,
	protocol.MethodInputText:  true,
	protocol.MethodBack:       true,
	protocol.MethodHome:       true,
	protocol.MethodRecents:    true,
	protocol.MethodClick:      true,
	protocol.MethodRightClick: true,
	protocol.MethodDrag:       true,
	protocol.MethodScroll:     true,
	protocol.MethodPressKeys:  true,
	protocol.MethodExecute:    true,
}

// Capturer takes one frame from a device on the recorder's behalf, for the
// follow-up after an action.
//
// An interface, so this package stays free of a relay import — the same reason
// Command exists rather than passing relay.FrameRecord around. It is also where
// the rails live: the implementation (in cmd/abacad) is the only layer that can
// see the hub AND the store, and it refuses to capture from a device that is
// offline, asleep, or not exposing the screenshot capability. Returning an error
// is normal and cheap; the caller drops the frame and never retries.
type Capturer interface {
	Capture(deviceID string) (protocol.ScreenshotResult, error)
}

// Command is one relayed command offered for recording. The caller builds it
// from relay.FrameRecord; keeping the relay type out of this package's signature
// mirrors how device.Handler translates a CommandRecord into a store.Activity,
// and keeps the decode work — which is the whole reason this is asynchronous —
// on this side of the channel.
type Command struct {
	AccountID  string
	DeviceID   string
	Ts         int64 // unix millis; stamped by Record when zero
	Method     string
	Source     string
	Outcome    string
	DurationMs int64
	ActorLabel string

	// Params is the command's parameters. ONLY numeric pointer coordinates are
	// read out of it (see marker); input_text's text and execute's code arrive
	// here too and are never persisted.
	Params map[string]any
	// Result is the device's raw reply, nil when the command failed.
	Result json.RawMessage
}

// Recorder is the async writer. Safe for concurrent use; a nil *Recorder is a
// no-op, so callers never need to guard.
type Recorder struct {
	st       *store.Store
	frames   *Frames
	ch       chan Command
	dropped  atomic.Uint64
	maxSteps int // per-device step cap; <= 0 disables the trim

	// Follow-up capture after an action. Both must be set for it to run.
	capturer Capturer
	settle   time.Duration

	mu sync.Mutex
	// pending is the one in-flight follow-up per device. A second action during
	// the wait resets this timer rather than stacking another capture: an agent
	// that taps three times in a row wants one picture of the result, not three.
	pending map[string]*time.Timer
	// frameSeq counts the frames recorded for each device, and lastFrameHash is
	// what was in the most recent one. The counter coalesces a follow-up away when
	// a frame arrived while it was waiting; the hash drops a follow-up whose
	// screen is byte-identical to the frame before it.
	//
	// A COUNTER, not a timestamp. The question is "has a frame been recorded since
	// this capture was scheduled", which is about ordering, not about time — and
	// asking it with unix-milli timestamps gets it wrong whenever the frame before
	// the action shares its millisecond, which on a fast local device is most of
	// the time. A monotonic per-device tick answers exactly the question, and is
	// immune to clock granularity and clock changes alike.
	//
	// In memory rather than columns on purpose: both are optimizations whose
	// worst case after a restart is one extra frame, which is not worth a
	// migration or a query on every write.
	frameSeq      map[string]uint64
	lastFrameHash map[string][sha256.Size]byte
}

// Option configures a Recorder at construction, before its goroutines start.
// Mirrors activity.Option.
type Option func(*Recorder)

// WithCapturer enables the follow-up frame after an action: settle after the
// command completes, then take a screenshot unless one has already landed. A nil
// capturer or a non-positive settle leaves it off, which is exactly the
// behaviour before this existed — actions are recorded as frameless steps.
func WithCapturer(c Capturer, settle time.Duration) Option {
	return func(r *Recorder) {
		if c == nil || settle <= 0 {
			return
		}
		r.capturer, r.settle = c, settle
	}
}

// New starts a Recorder writing steps to st and frames to fr. retention <= 0
// disables the age sweep (keep forever, matching every other retention knob);
// maxSteps <= 0 disables the per-device cap. Both sweeps run once at start and
// then every gcInterval. Options are applied before any goroutine starts.
func New(st *store.Store, fr *Frames, retention time.Duration, maxSteps int, opts ...Option) *Recorder {
	r := &Recorder{
		st: st, frames: fr, ch: make(chan Command, 512), maxSteps: maxSteps,
		pending:       make(map[string]*time.Timer),
		frameSeq:      make(map[string]uint64),
		lastFrameHash: make(map[string][sha256.Size]byte),
	}
	for _, opt := range opts {
		opt(r)
	}
	go r.writeLoop()
	go r.reportDrops()
	go r.gcLoop(retention)
	return r
}

// Record enqueues one command for recording. Never blocks.
//
// Dashboard-sourced commands are dropped here rather than in the write loop: the
// dashboard's live view captures a frame every two seconds for as long as a
// device page is open, which would bury the agent's own frames under an order of
// magnitude more of them — and nobody replays a session they were watching live.
func (r *Recorder) Record(c Command) {
	if r == nil || c.Source == "dashboard" {
		return
	}
	if c.Ts == 0 {
		c.Ts = time.Now().UnixMilli()
	}
	select {
	case r.ch <- c:
	default:
		// Full — drop rather than stall the relay, but count it. Unlike the
		// activity trail, a gap here is visible to the person watching (the
		// timeline just skips), so this is the less costly of the two failures.
		r.dropped.Add(1)
	}
}

// Dropped is the number of steps lost to a full buffer since start. Non-zero
// means some sessions have gaps.
func (r *Recorder) Dropped() uint64 {
	if r == nil {
		return 0
	}
	return r.dropped.Load()
}

// Forget removes every recorded step and frame for a device. Called when the
// device is deleted: its frames are pictures of its screen, and deleting the
// device must not leave them behind.
func (r *Recorder) Forget(deviceID string) {
	if r == nil {
		return
	}
	ids, err := r.st.DeleteReplayDevice(deviceID)
	r.frames.Delete(ids...)
	if err != nil {
		log.Printf("[replay] forget device=%s: %v", deviceID, err)
	}
}

func (r *Recorder) reportDrops() {
	var last uint64
	for {
		time.Sleep(dropReportInterval)
		if n := r.dropped.Load(); n > last {
			log.Printf("[replay] dropped %d step(s) — recorder buffer full, sessions have gaps (total %d)", n-last, n)
			last = n
		}
	}
}

func (r *Recorder) writeLoop() {
	for c := range r.ch {
		r.write(c)
	}
}

// write persists one command: its frames first (so a row never points at bytes
// that aren't there yet), then its rows. A command with no image still gets a
// row — a tap is part of the story even though it draws nothing — and, when
// follow-up capture is on, schedules the picture of what it did.
func (r *Recorder) write(c Command) {
	shots := framesOf(c)
	if len(shots) == 0 {
		r.insert(c, store.ReplayStep{Marker: marker(c.Method, c.Params)})
		r.scheduleCapture(c)
		return
	}
	for _, s := range shots {
		jpeg, err := base64.StdEncoding.DecodeString(s.PNGBase64)
		if err != nil || len(jpeg) == 0 {
			// A frame we can't decode is not worth a row that claims to have one;
			// record the step without it so the timeline still shows the command.
			r.insert(c, store.ReplayStep{Marker: marker(c.Method, c.Params)})
			continue
		}
		// A follow-up that found the screen exactly as the last frame left it has
		// nothing to add, so it is dropped whole — no row, no file. Only OUR
		// captures are dropped this way: an agent's own screenshot is a command it
		// really issued, and the timeline has to show it whether or not the pixels
		// moved.
		sum := sha256.Sum256(jpeg)
		if c.Source == SourceReplay && r.sameAsLastFrame(c.DeviceID, sum) {
			continue
		}
		id := auth.NewID("frm")
		if err := r.frames.Save(id, jpeg); err != nil {
			log.Printf("[replay] save frame device=%s: %v", c.DeviceID, err)
			r.insert(c, store.ReplayStep{Marker: marker(c.Method, c.Params)})
			continue
		}
		if !r.insert(c, store.ReplayStep{
			FrameID: id, W: s.W, H: s.H, Marker: marker(c.Method, c.Params),
		}) {
			// No row means nothing will ever find this file again, and nothing
			// will ever delete it either. Take it back out now.
			r.frames.Delete(id)
			continue
		}
		r.noteFrame(c.DeviceID, sum)
	}
}

// noteFrame records that a device's screen was captured, for the two rules that
// keep follow-ups from doubling every frame.
func (r *Recorder) noteFrame(deviceID string, sum [sha256.Size]byte) {
	r.mu.Lock()
	r.frameSeq[deviceID]++
	r.lastFrameHash[deviceID] = sum
	r.mu.Unlock()
}

// sameAsLastFrame reports whether a device's screen is byte-identical to the
// frame already recorded for it.
func (r *Recorder) sameAsLastFrame(deviceID string, sum [sha256.Size]byte) bool {
	r.mu.Lock()
	prev, ok := r.lastFrameHash[deviceID]
	r.mu.Unlock()
	return ok && prev == sum
}

// scheduleCapture arranges the picture of what an action did: wait for the UI to
// settle, then take a frame — unless one has arrived in the meantime.
//
// The wait is what makes this cheap. An agent's own next screenshot usually
// follows within a second or two, and when it does, that IS the after-picture and
// this fires nothing. What is left is the case the feature exists for: an agent
// that acts several times without looking, where nothing else will ever produce
// a picture of the result.
//
// Only successful actions are followed. A tap that timed out or was denied by
// the capability gate changed nothing, so there is nothing new to photograph.
func (r *Recorder) scheduleCapture(c Command) {
	if r.capturer == nil || c.Outcome != "ok" || !actionVerbs[protocol.Method(c.Method)] {
		return
	}
	r.mu.Lock()
	// One pending capture per device: reset rather than stack, so a burst of
	// actions yields one picture of where the burst ended.
	if t := r.pending[c.DeviceID]; t != nil {
		t.Stop()
	}
	// Snapshot the device's frame count NOW: everything recorded up to this point
	// precedes the action, so only a tick AFTER this one means the result has
	// already been photographed by somebody else.
	seq := r.frameSeq[c.DeviceID]
	r.pending[c.DeviceID] = time.AfterFunc(r.settle, func() { r.capture(c, seq) })
	r.mu.Unlock()
}

// capture takes the follow-up frame and feeds it back through the normal
// recording path, tagged SourceReplay.
//
// Everything here is best-effort and silent on failure. A device that went
// offline, fell asleep, or had screenshot turned off between the action and now
// is an ordinary outcome, not an error worth a log line per action — and it is
// never retried, because by the time a retry landed the screen would no longer
// be the result of the action that triggered it.
func (r *Recorder) capture(c Command, scheduledAt uint64) {
	r.mu.Lock()
	delete(r.pending, c.DeviceID)
	seq := r.frameSeq[c.DeviceID]
	r.mu.Unlock()

	// A frame was recorded while we waited — the agent's own screenshot, or an
	// earlier follow-up. That IS the after-picture; nothing to add.
	if seq != scheduledAt {
		return
	}
	shot, err := r.capturer.Capture(c.DeviceID)
	if err != nil || shot.PNGBase64 == "" {
		return
	}
	shot.Tree = nil // never stored; see framesOf
	result, err := json.Marshal(shot)
	if err != nil {
		return
	}
	// Back through Record rather than straight to write: it keeps a single
	// writer, and it means a follow-up is subject to the same backpressure as
	// everything else — under load the picture is what should be dropped, not the
	// agent's own record. Method is screenshot, which is not an action verb, so
	// this cannot schedule a further capture.
	r.Record(Command{
		AccountID: c.AccountID, DeviceID: c.DeviceID,
		Method: string(protocol.MethodScreenshot), Source: SourceReplay, Outcome: "ok",
		Result: result,
	})
}

// insert fills the common columns from c and writes the row. Returns whether it
// landed, so a caller that already wrote a frame file can undo it.
func (r *Recorder) insert(c Command, st store.ReplayStep) bool {
	st.AccountID = c.AccountID
	st.DeviceID = c.DeviceID
	st.Ts = c.Ts
	st.Method = c.Method
	st.Source = c.Source
	st.Outcome = c.Outcome
	st.DurationMs = c.DurationMs
	st.ActorLabel = c.ActorLabel
	if err := r.st.InsertReplayStep(st); err != nil {
		log.Printf("[replay] insert failed (device=%s method=%s): %v", c.DeviceID, c.Method, err)
		return false
	}
	return true
}

// framesOf pulls the images out of a command's reply. Only two verbs carry any:
// screenshot returns one, composite returns one per screenshot step in its
// sequence. Everything else — including any failed command, whose Result is nil
// — yields none.
func framesOf(c Command) []protocol.ScreenshotResult {
	if len(c.Result) == 0 || c.Outcome != "ok" {
		return nil
	}
	switch protocol.Method(c.Method) {
	case protocol.MethodScreenshot:
		var s protocol.ScreenshotResult
		if json.Unmarshal(c.Result, &s) != nil || s.PNGBase64 == "" {
			return nil
		}
		// The UI tree rides along in the reply and is not kept: it is the agent's
		// working data, often larger than the JPEG, and adds nothing to a picture
		// of the screen it already describes.
		s.Tree = nil
		return []protocol.ScreenshotResult{s}
	case protocol.MethodComposite:
		var res protocol.CompositeResult
		if json.Unmarshal(c.Result, &res) != nil {
			return nil
		}
		out := make([]protocol.ScreenshotResult, 0, len(res.Shots))
		for _, s := range res.Shots {
			if s.PNGBase64 != "" {
				s.Tree = nil
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// marker renders a pointer command's coordinates as the JSON the player draws
// over the preceding frame, or "" for a command with no position.
//
// The allowlist is the point. Params also carries typed text, evaluated
// JavaScript, shell arguments and file paths; a denylist would keep every one of
// those the day someone adds a verb. Only these keys, only as numbers.
func marker(method string, params map[string]any) string {
	if params == nil {
		return ""
	}
	type point struct {
		Kind string `json:"kind"`
		X    int    `json:"x"`
		Y    int    `json:"y"`
		X2   *int   `json:"x2,omitempty"`
		Y2   *int   `json:"y2,omitempty"`
	}
	num := func(k string) (int, bool) {
		switch v := params[k].(type) {
		case int:
			return v, true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		default:
			return 0, false
		}
	}
	var m point
	switch protocol.Method(method) {
	case protocol.MethodTap, protocol.MethodLongPress, protocol.MethodClick,
		protocol.MethodRightClick, protocol.MethodScroll:
		x, okX := num("x")
		y, okY := num("y")
		if !okX || !okY {
			return ""
		}
		m = point{Kind: method, X: x, Y: y}
	case protocol.MethodSwipe, protocol.MethodDrag:
		x1, ok1 := num("x1")
		y1, ok2 := num("y1")
		x2, ok3 := num("x2")
		y2, ok4 := num("y2")
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return ""
		}
		m = point{Kind: method, X: x1, Y: y1, X2: &x2, Y2: &y2}
	default:
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// gcLoop enforces both bounds on recorded data: the retention window, and the
// per-device step cap that keeps one busy agent from filling the disk inside
// that window. Rows go first and their frames follow, then any file the window
// says must be gone regardless of what the database knows (see sweepOrphans).
func (r *Recorder) gcLoop(retention time.Duration) {
	for {
		if retention > 0 {
			cutoff := time.Now().Add(-retention).UnixMilli()
			ids, err := r.st.PruneReplayStepsBefore(cutoff)
			r.frames.Delete(ids...)
			if err != nil {
				log.Printf("[replay] prune failed: %v", err)
			} else if len(ids) > 0 {
				log.Printf("[replay] pruned %d frame(s) past retention", len(ids))
			}
		}
		if r.maxSteps > 0 {
			devices, err := r.st.ReplayDeviceIDs()
			if err != nil {
				log.Printf("[replay] trim: list devices: %v", err)
			}
			for _, id := range devices {
				ids, err := r.st.TrimReplayDevice(id, r.maxSteps)
				r.frames.Delete(ids...)
				if err != nil {
					log.Printf("[replay] trim device=%s: %v", id, err)
				} else if len(ids) > 0 {
					log.Printf("[replay] trimmed %d frame(s) past the cap on device=%s", len(ids), id)
				}
			}
		}
		if n := r.frames.sweepOrphans(retention); n > 0 {
			log.Printf("[replay] swept %d orphaned frame file(s)", n)
		}
		time.Sleep(gcInterval)
	}
}
