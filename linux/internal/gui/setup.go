package gui

import "abacad-linux/internal/enroll"

// Setup is what the GUI needs to run self-enrollment itself, rather than having
// the caller block on it before the window exists. Declared outside the build
// tags so both the real GTK build and the headless stub share one signature.
type Setup struct {
	// Session is the shared register -> claim state machine, pre-wired by the
	// caller with the relay, the device name, and config persistence.
	Session *enroll.Session

	// Token and DeviceID are the credential already on disk, both blank on a
	// fresh install. A non-blank Token means a human completed setup at some
	// point, so the GUI resumes automatically; blank means it waits for the
	// Set up button.
	Token    string
	DeviceID string

	// Supported is false where self-enrollment cannot work — a box with no
	// display has no screen to read a claim code off. The GUI then points at
	// `abacad connect` instead of offering a button that cannot succeed.
	Supported bool

	// DialURL assembles the websocket URL from a claimed device's relay URL and
	// token. Supplied by the caller so the GUI shares the daemon's assembly —
	// which appends ?version= as well as ?token=, and is what makes the client
	// version show up in the dashboard's device list.
	DialURL func(deviceURL, token string) string
}

// Configured reports whether this device has been set up before, and so may
// reconnect without asking.
func (s Setup) Configured() bool { return s.Token != "" }

// wired reports whether the caller supplied enough to drive enrollment at all. A
// zero Setup means it did not; the window still opens and falls back to the
// manual URL entry rather than panicking on a nil deref in a GTK callback.
func (s Setup) wired() bool { return s.Session != nil && s.DialURL != nil }

// canEnroll reports whether the setup button can succeed. Registering needs a
// screen to read the claim code off, which Supported answers.
func (s Setup) canEnroll() bool { return s.wired() && s.Supported }

// canAct reports whether the setup button can do anything at all: register on a
// box with a screen, or resume an existing credential on one without.
func (s Setup) canAct() bool { return s.canEnroll() || s.canResume() }

// canResume reports whether to reconnect silently at startup. Deliberately does
// NOT consult Supported: resuming an existing credential displays no claim code,
// so it works on a session where registering wouldn't. Supported is decided by
// an X11 probe, and gating resume on it would strand every already-enrolled
// device on a Wayland session that GTK renders on perfectly well.
func (s Setup) canResume() bool { return s.wired() && s.Configured() }

// relay names the relay the setup button would contact, for display before it is
// contacted. Safe on a zero Setup.
func (s Setup) relay() string {
	if s.Session == nil {
		return ""
	}
	return s.Session.Relay
}
