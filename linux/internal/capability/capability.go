// Package capability is the device-side ceiling: which interfaces this machine
// is willing to expose, decided here rather than by the relay.
//
// The server has its own per-device capability set, and the effective surface is
// the intersection of the two. The difference between them is who you have to
// trust. The server's set protects you from a misbehaving agent; this one
// protects you from a misbehaving *server* — a relay that is compromised,
// misconfigured, or simply run by someone else cannot switch a capability back
// on here, because this process refuses the command regardless of what it is
// told. That is the whole point of the split, and it is why the check lives in
// front of the dispatcher rather than being trusted to the wire.
//
// Honest limits, worth knowing before relying on this:
//
//   - It cannot constrain a capability that already grants code execution as
//     this user. The config below is a plain file; an agent permitted to write
//     files can rewrite it, and one permitted to type into a terminal can do
//     worse. Turning "files" off is meaningful; leaving it on and expecting the
//     rest to hold is not.
//   - It is not a sandbox. A capability that is on is on completely — this
//     decides *whether*, never *how much*.
package capability

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// The capability vocabulary, mirroring the server's protocol.Capabilities. The
// names are the protocol verbs, plus the three that ride the tunnel lane or are
// server-initiated and so have no verb of their own.
const (
	Screenshot      = "screenshot"
	Tap             = "tap"
	LongPress       = "long_press"
	Swipe           = "swipe"
	InputText       = "input_text"
	Back            = "back"
	Home            = "home"
	Recents         = "recents"
	Click           = "click"
	RightClick      = "right_click"
	Drag            = "drag"
	Scroll          = "scroll"
	PressKeys       = "press_keys"
	Composite       = "composite"
	Execute         = "execute"
	PushFile        = "push_file"
	PullFile        = "pull_file"
	ScreenRecording = "screen_recording"
	Tunnel          = "tunnel"
	SSH             = "ssh"
	VNC             = "vnc"
)

// All is every capability this client understands, in the server's order.
var All = []string{
	Screenshot, Tap, LongPress, Swipe, InputText, Back, Home, Recents,
	Click, RightClick, Drag, Scroll, PressKeys, Composite,
	Execute, PushFile, PullFile, ScreenRecording,
	Tunnel, SSH, VNC,
}

// Wildcard means "everything, including capabilities added in later versions".
// An unconfigured client reports this rather than enumerating All, so it does
// not pin itself to the verb list of the version it shipped with.
const Wildcard = "*"

// ErrDisabled is returned when the local configuration does not expose a
// capability. The agent maps it to the wire's capability_disabled code so the
// refusal is machine-distinguishable from "unknown method".
var ErrDisabled = fmt.Errorf("disabled on this device")

var (
	mu        sync.RWMutex
	wildcard  = true // no config yet => expose everything, as before this existed
	enabled   = map[string]bool{}
	listeners []func()
)

// Allows reports whether this device exposes name.
func Allows(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return wildcard || enabled[name]
}

// Check returns ErrDisabled (wrapped, naming the capability) when name is off.
func Check(name string) error {
	if Allows(name) {
		return nil
	}
	return fmt.Errorf("%s is %w — turn it back on in the abacad app on this device", name, ErrDisabled)
}

// Report is what this device advertises to the server: the full set, always, so
// the most recent frame is the whole truth and no delta can drift. The wildcard
// stays a wildcard rather than being expanded, so a newer server keeps granting
// capabilities this build has never heard of.
func Report() []string {
	mu.RLock()
	defer mu.RUnlock()
	if wildcard {
		return []string{Wildcard}
	}
	out := make([]string, 0, len(enabled))
	for _, name := range All {
		if enabled[name] {
			out = append(out, name)
		}
	}
	return out
}

// Set replaces the enabled set and notifies listeners (the agent re-reports to
// the server; the GUI repaints). Passing Wildcard restores "everything".
func Set(names []string) {
	mu.Lock()
	wildcard = false
	enabled = map[string]bool{}
	for _, n := range names {
		if n == Wildcard {
			wildcard = true
			enabled = map[string]bool{}
			break
		}
		enabled[n] = true
	}
	subs := append([]func(){}, listeners...)
	mu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// Toggle turns one capability on or off and notifies listeners. Toggling off
// while in wildcard mode first materializes the wildcard into the concrete set,
// so "everything except X" is expressible.
func Toggle(name string, on bool) {
	mu.Lock()
	if wildcard {
		wildcard = false
		enabled = map[string]bool{}
		for _, n := range All {
			enabled[n] = true
		}
	}
	if on {
		enabled[name] = true
	} else {
		delete(enabled, name)
	}
	subs := append([]func(){}, listeners...)
	mu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// Enabled lists the currently exposed capabilities, expanding the wildcard, for
// a UI that wants concrete checkboxes.
func Enabled() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(All))
	for _, name := range All {
		if wildcard || enabled[name] {
			out = append(out, name)
		}
	}
	return out
}

// OnChange registers a callback fired after any change. Callbacks run outside
// the lock, so they may call back into this package.
func OnChange(fn func()) {
	mu.Lock()
	listeners = append(listeners, fn)
	mu.Unlock()
}

// configPath is ~/.config/abacad/capabilities — a file of its own rather than a
// key in the main config, which holds connection credentials and is rewritten
// wholesale on re-enrollment.
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "abacad", "capabilities"), nil
}

// Load reads the persisted set. A missing file leaves the wildcard in place, so
// an existing install keeps behaving exactly as it did before this feature.
func Load() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no config => wildcard
		}
		return err
	}
	defer f.Close()

	var names []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names = append(names, line)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// A file that exists but lists nothing means "expose nothing" — distinct
	// from no file at all. Set(nil) gives exactly that.
	Set(names)
	return nil
}

// Save persists the current set, one capability per line.
func Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	mu.RLock()
	var lines []string
	if wildcard {
		lines = []string{Wildcard}
	} else {
		for name := range enabled {
			lines = append(lines, name)
		}
		sort.Strings(lines)
	}
	mu.RUnlock()

	body := "# abacad: what this device exposes. One capability per line.\n" +
		"# \"*\" means everything, including capabilities added in future versions.\n" +
		"# An empty list means expose nothing.\n" +
		strings.Join(lines, "\n") + "\n"
	// 0600: not a secret, but it is a policy decision about this machine, and it
	// should not be world-writable.
	return os.WriteFile(path, []byte(body), 0o600)
}
