// Package status is the abacad Linux client's in-process connection + activity
// state, shared between the agent (the writer) and the GUI (the reader). It is
// the Go analogue of the Android client's AbacadStatus: a plain, thread-safe
// singleton with a small ring of activity lines and change listeners.
//
// It stays in the cgo-free core (no GTK), so the headless daemon links it too —
// there it's just structured state behind the log lines. The GUI subscribes and
// repaints; a headless build simply never reads it.
package status

import (
	"sync"
	"time"
)

// State is the coarse connection state that drives the panel headline + color.
type State int

const (
	Disconnected State = iota
	Connecting
	Connected
	Reconnecting
)

func (s State) String() string {
	switch s {
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Reconnecting:
		return "reconnecting"
	default:
		return "disconnected"
	}
}

// Line is one timestamped activity entry (a state change, a command, an error).
type Line struct {
	TS   int64 // unix millis
	Text string
}

// Snapshot is an immutable view of the whole state, handed to the GUI so it can
// render without holding the lock.
type Snapshot struct {
	State       State
	Detail      string
	Paused      bool
	Watched     bool
	Recording   bool
	Controlling bool
	LastMethod  string
	Lines       []Line
}

const (
	maxLines            = 40
	controllingWindowMs = 6000
)

var (
	mu            sync.Mutex
	state         = Disconnected
	detail        = "not connected"
	paused        bool
	watched       bool
	recording     bool
	lastCommandAt int64
	lastMethod    string
	lines         []Line
	listeners     = map[int]func(){}
	nextID        int
)

func nowMs() int64 { return time.Now().UnixMilli() }

// SetState moves to s with a one-line detail, recorded as an activity line too.
func SetState(s State, d string) {
	mu.Lock()
	state, detail = s, d
	appendLocked("• " + d)
	mu.Unlock()
	notify()
}

// Event records a discrete activity line without changing state.
func Event(text string) {
	mu.Lock()
	appendLocked(text)
	mu.Unlock()
	notify()
}

// SetPaused toggles the soft-kill pause (from the GUI). While paused the client
// stays connected but the agent rejects every command locally.
func SetPaused(p bool) {
	mu.Lock()
	if paused == p {
		mu.Unlock()
		return
	}
	paused = p
	if p {
		appendLocked("⏸ control paused by device operator")
	} else {
		appendLocked("▶ control resumed")
	}
	mu.Unlock()
	notify()
}

// Paused reports whether control is currently paused (read on the command path).
func Paused() bool {
	mu.Lock()
	defer mu.Unlock()
	return paused
}

// SetWatched marks whether a live-view (VNC) session is active.
func SetWatched(w bool) {
	mu.Lock()
	if watched == w {
		mu.Unlock()
		return
	}
	watched = w
	if w {
		appendLocked("👁 live view started — screen being watched")
	} else {
		appendLocked("live view ended")
	}
	mu.Unlock()
	notify()
}

// SetRecording marks whether a screen recording is in progress.
func SetRecording(r bool) {
	mu.Lock()
	if recording == r {
		mu.Unlock()
		return
	}
	recording = r
	if r {
		appendLocked("● screen recording started")
	} else {
		appendLocked("screen recording stopped")
	}
	mu.Unlock()
	notify()
}

// NoteCommand records that a command arrived (drives the "controlling now" state).
func NoteCommand(method string) {
	mu.Lock()
	lastCommandAt = nowMs()
	lastMethod = method
	mu.Unlock()
	notify()
}

// Get returns an immutable snapshot for rendering.
func Get() Snapshot {
	mu.Lock()
	defer mu.Unlock()
	cp := make([]Line, len(lines))
	copy(cp, lines)
	return Snapshot{
		State:       state,
		Detail:      detail,
		Paused:      paused,
		Watched:     watched,
		Recording:   recording,
		Controlling: state == Connected && !paused && nowMs()-lastCommandAt < controllingWindowMs,
		LastMethod:  lastMethod,
		Lines:       cp,
	}
}

// Subscribe registers fn, called (on the writer's goroutine) after every change.
// It returns an unsubscribe function. The GUI marshals fn onto the GTK thread.
func Subscribe(fn func()) func() {
	mu.Lock()
	id := nextID
	nextID++
	listeners[id] = fn
	mu.Unlock()
	return func() {
		mu.Lock()
		delete(listeners, id)
		mu.Unlock()
	}
}

func appendLocked(text string) {
	lines = append(lines, Line{TS: nowMs(), Text: text})
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
}

func notify() {
	mu.Lock()
	fns := make([]func(), 0, len(listeners))
	for _, fn := range listeners {
		fns = append(fns, fn)
	}
	mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}
