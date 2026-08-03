package enroll

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Session drives the whole register -> wait-to-be-claimed -> dial state machine
// described in the package doc, reporting progress through callbacks so the
// daemon can print an ASCII box to a terminal and the GUI can update labels from
// the same code. It was lifted out of cmd/abacad for exactly that reason: the GUI
// lives in internal/, which cannot reach into package main.
//
// It performs no I/O of its own beyond the relay calls — persisting credentials
// is the caller's job, via Persist.
type Session struct {
	// Relay is the base URL to enroll with; blank means DefaultRelay.
	Relay string
	// Platform, Name and Version are reported at registration and shown to
	// whoever claims the device.
	Platform string
	Name     string
	Version  string

	// Persist stores a freshly issued credential, and is called with empty
	// deviceID/token to wipe one the relay has rejected. Run aborts if it fails
	// on the store path: the relay has already minted a device row, and losing
	// the token would orphan it and re-register on every start.
	Persist func(relay, deviceID, token string) error

	// OnCode reports the claim code to display, and is called only when the code
	// actually changes — the relay rotates it, and neither a terminal nor a GTK
	// label wants the repaint churn of every heartbeat.
	OnCode func(deviceID, code string)

	// OnStatus reports human-readable progress ("re-registering", "claimed by
	// …"). Optional; nil discards.
	OnStatus func(message string)

	// retryDelay is the pause between heartbeat retries; zero means 10s. Exists
	// so the retry-cap test doesn't have to sleep for a minute and a half.
	retryDelay time.Duration
}

// maxTransient is how many consecutive heartbeat failures to absorb before
// giving up. Matches the other clients; the point is that a caller showing
// "Contacting…" eventually gets told it isn't working.
const maxTransient = 10

// Result is a completed enrollment: everything needed to dial.
type Result struct {
	DeviceURL string // wss://relay/device
	Token     string
	DeviceID  string
	ClaimedBy string // account that claimed it; "" when the relay didn't say
}

// ErrSetupRequired means this device has no credential and Run was called
// without permission to create one. It is not a failure — it is the signal that
// a human has to ask for setup first. Callers surface a button, not an error.
//
// Registering is the one step that contacts a relay we have never spoken to and
// tells it this machine's hostname, so it happens on request rather than on
// launch. See docs/trust.md.
var ErrSetupRequired = errors.New("device is not set up yet")

// errRestart signals internally that a stored credential was rejected and the
// loop should re-enter registration.
var errRestart = errors.New("credentials rejected; re-enrollment required")

// Run resolves this machine's credentials, blocking until a human claims the
// device. Pass the stored token and deviceID (both blank on a fresh install).
//
// allowRegister gates the one call that creates a device row on the relay. Pass
// false on any path that runs without a human having asked — app launch, service
// restart — and Run returns ErrSetupRequired instead of registering. Pass true
// only from an explicit action. The flag is consulted on every pass of the loop,
// not just the first, because a credential rejected mid-flight sends us back
// through Register, and that second registration needs the same consent as the
// first.
func (s *Session) Run(ctx context.Context, token, deviceID string, allowRegister bool) (*Result, error) {
	relay := New(s.Relay).Relay // normalize once, so Persist and DeviceURL agree
	c := New(relay)

	// Two passes at most. The second exists for exactly one case: the stored
	// credential turned out to be dead (device deleted, registration reaped), so
	// we discard it and enroll from scratch in-process rather than making the
	// operator restart. Bounded so a systematically rejecting relay can't spin.
	for attempt := 0; attempt < 2; attempt++ {
		if token == "" {
			if !allowRegister {
				return nil, ErrSetupRequired
			}
			reg, err := c.Register(s.Platform, s.Name, s.Version)
			if err != nil {
				// ErrNotSupported passes through unwrapped so callers can offer
				// the `abacad connect` fallback with errors.Is.
				if errors.Is(err, ErrNotSupported) {
					return nil, err
				}
				return nil, fmt.Errorf("register with %s: %w", relay, err)
			}
			token, deviceID = reg.DeviceToken, reg.DeviceID
			if s.Persist != nil {
				if err := s.Persist(relay, deviceID, token); err != nil {
					return nil, fmt.Errorf("save credentials: %w", err)
				}
			}
			if s.OnCode != nil && reg.ClaimCode != "" {
				s.OnCode(deviceID, reg.ClaimCode)
			}
		}

		st, err := s.waitForClaim(ctx, c, relay, token, deviceID)
		switch {
		case errors.Is(err, errRestart):
			token, deviceID = "", "" // credential wiped; loop back into Register
			continue
		case err != nil:
			return nil, err
		case st == nil:
			// Cancelled. ctx.Err() is normally non-nil here, but sleepCtx's
			// select can pick the timer when both cases are ready, so never
			// return a nil error with a nil result — callers dereference it.
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			return nil, context.Canceled
		}

		deviceURL, err := DeviceURL(relay)
		if err != nil {
			return nil, err
		}
		return &Result{
			DeviceURL: deviceURL,
			Token:     token,
			DeviceID:  firstNonEmpty(st.DeviceID, deviceID),
			ClaimedBy: st.ClaimedBy,
		}, nil
	}
	return nil, errors.New("relay rejected freshly issued credentials twice — giving up")
}

// waitForClaim polls until a human claims this device, reporting the id and the
// current claim code through OnCode whenever the code changes.
func (s *Session) waitForClaim(ctx context.Context, c *Client, relay, token, deviceID string) (*Status, error) {
	var lastCode string
	var transient int
	for {
		st, err := c.Heartbeat(token)
		if errors.Is(err, ErrUnknownToken) {
			// The registration was reaped or the device deleted. Wipe and start
			// over rather than heartbeating a dead credential forever.
			s.status("relay no longer recognizes this device — re-registering")
			if s.Persist != nil {
				if err := s.Persist(relay, "", ""); err != nil {
					s.status(fmt.Sprintf("could not clear stale credentials: %v", err))
				}
			}
			return nil, errRestart
		}
		if err != nil {
			// A flaky network shouldn't abandon a setup screen someone is reading
			// a code off, but it must not retry forever either: a GUI caller
			// leaves its "Contacting…" state up for as long as this blocks, with
			// no button and no explanation. Same cap as the other clients.
			transient++
			if transient >= maxTransient {
				return nil, fmt.Errorf("lost contact with %s while waiting to be claimed: %w", relay, err)
			}
			s.status(fmt.Sprintf("heartbeat: %v — retrying", err))
			delay := s.retryDelay
			if delay <= 0 {
				delay = 10 * time.Second
			}
			if !sleepCtx(ctx, delay) {
				return nil, nil
			}
			continue
		}
		transient = 0

		if st.Claimed {
			owner := st.ClaimedBy
			if owner == "" {
				owner = "your account"
			}
			// Disclose WHO claimed it. If this is a name you don't recognize,
			// someone who saw your screen claimed this device — see docs/abuse.md.
			s.status(fmt.Sprintf("claimed by %s — connecting", owner))
			return st, nil
		}

		if st.ClaimCode != lastCode {
			if s.OnCode != nil {
				s.OnCode(firstNonEmpty(st.DeviceID, deviceID), st.ClaimCode)
			}
			lastCode = st.ClaimCode
		}
		if !sleepCtx(ctx, Interval(st.HeartbeatIn)) {
			return nil, nil
		}
	}
}

func (s *Session) status(msg string) {
	if s.OnStatus != nil {
		s.OnStatus(msg)
	}
}

// sleepCtx waits for d, reporting false if the context was cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
