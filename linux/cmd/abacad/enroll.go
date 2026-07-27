package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"abacad-linux/internal/enroll"
	"abacad-linux/internal/version"
)

// Self-enrollment wiring for the daemon. The package doc in internal/enroll
// describes the state machine; this file is the part that touches the config
// file, the terminal, and the process lifetime.
//
// Headless boxes deliberately do NOT self-enroll. The whole premise is that a
// human reads an id and a claim code off the device's own screen, and a rack
// server has none — so `abacad connect` (RFC 8628, approved in a browser
// elsewhere) remains the enrollment path there. detectPlatform() already
// computes exactly that distinction for the pairing flow; we reuse it.

// enrolled is the outcome of ensureEnrolled: everything the daemon needs to dial.
type enrolled struct {
	deviceURL string // wss://relay/device
	token     string
	deviceID  string
}

// ensureEnrolled resolves this machine's device credentials, self-registering
// with the relay and blocking until a human claims it if necessary.
//
// It returns nil (with no error) when self-enrollment does not apply — headless,
// or a relay too old to support it — so the caller can fall back to the legacy
// configured-URL path and print the appropriate guidance.
func ensureEnrolled(ctx context.Context, cfg map[string]string, relayFlag string) (*enrolled, error) {
	if detectPlatform() == "linux-headless" {
		return nil, nil
	}
	relay := firstNonEmpty(relayFlag, os.Getenv("ABACAD_RELAY_URL"), cfg["relay_url"], enroll.DefaultRelay)
	c := enroll.New(relay)

	token := cfg["device_token"]
	deviceID := cfg["device_id"]

	// Two passes at most. The second exists for exactly one case: the stored
	// credential turned out to be dead (device deleted, registration reaped), so
	// we discard it and enroll from scratch in-process rather than making the
	// operator restart. Bounded so a systematically rejecting relay can't spin.
	for attempt := 0; attempt < 2; attempt++ {
		if token == "" {
			reg, err := c.Register(detectPlatform(), defaultDeviceName(), version.Version)
			if errors.Is(err, enroll.ErrNotSupported) {
				log.Printf("relay %s does not support self-enrollment — falling back to `abacad connect`", relay)
				return nil, nil
			}
			if err != nil {
				return nil, fmt.Errorf("register with %s: %w", relay, err)
			}
			token, deviceID = reg.DeviceToken, reg.DeviceID
			if err := persist(relay, deviceID, token); err != nil {
				// Refuse to continue with a credential we failed to save: the relay
				// already minted a device row, and forgetting the token would orphan
				// it and re-register on every start.
				return nil, fmt.Errorf("save credentials: %w", err)
			}
		}

		status, err := waitForClaim(ctx, c, token, deviceID, relay)
		if errors.Is(err, errRestartEnrollment) {
			token, deviceID = "", "" // credential wiped; loop back into Register
			continue
		}
		if err != nil {
			return nil, err
		}
		if status == nil { // context cancelled
			return nil, ctx.Err()
		}

		deviceURL, err := enroll.DeviceURL(relay)
		if err != nil {
			return nil, err
		}
		return &enrolled{deviceURL: deviceURL, token: token, deviceID: status.DeviceID}, nil
	}
	return nil, errors.New("relay rejected freshly issued credentials twice — giving up")
}

// waitForClaim polls until a human claims this device, showing the id and the
// current claim code. It re-prints only when the code actually changes, so the
// terminal (and the journal) doesn't fill with duplicates during the wait.
func waitForClaim(ctx context.Context, c *enroll.Client, token, deviceID, relay string) (*enroll.Status, error) {
	var lastCode string
	for {
		st, err := c.Heartbeat(token)
		if errors.Is(err, enroll.ErrUnknownToken) {
			// The registration was reaped or the device deleted. Wipe and start
			// over rather than heartbeating a dead credential forever.
			log.Printf("relay no longer recognizes this device — re-registering")
			if err := persist(relay, "", ""); err != nil {
				log.Printf("could not clear stale credentials: %v", err)
			}
			return nil, errRestartEnrollment
		}
		if err != nil {
			log.Printf("heartbeat: %v — retrying", err)
			if !sleepCtx(ctx, 10*time.Second) {
				return nil, nil
			}
			continue
		}

		if st.Claimed {
			owner := st.ClaimedBy
			if owner == "" {
				owner = "your account"
			}
			// Disclose WHO claimed it. If this is a name you don't recognize,
			// someone who saw your screen claimed this device — see docs/abuse.md.
			log.Printf("claimed by %s — connecting", owner)
			return st, nil
		}

		if st.ClaimCode != lastCode {
			printClaimPrompt(st, deviceID, relay)
			lastCode = st.ClaimCode
		}
		if !sleepCtx(ctx, enroll.Interval(st.HeartbeatIn)) {
			return nil, nil
		}
	}
}

// errRestartEnrollment signals that the stored credential was rejected and the
// caller should re-enter enrollment from scratch.
var errRestartEnrollment = errors.New("credentials rejected; re-enrollment required")

// printClaimPrompt is the terminal equivalent of the GUI's setup screen: the two
// things a human needs to read off this machine, and where to type them.
func printClaimPrompt(st *enroll.Status, deviceID, relay string) {
	id := firstNonEmpty(st.DeviceID, deviceID)
	claimAt := strings.TrimRight(relay, "/") + "/claim"
	fmt.Fprintf(os.Stderr, `
  ┌─ Add this device ─────────────────────────────
  │
  │   Device ID    %s
  │   Claim code   %s
  │
  │   Open %s
  │   and enter both to add this device to your account.
  │
  │   The code changes every few minutes; anyone who can
  │   read both lines can claim this device.
  └───────────────────────────────────────────────

`, id, st.ClaimCode, claimAt)
}

// persist rewrites the config with the self-enrollment keys. Empty values clear
// the credential (used when the relay rejects it).
func persist(relay, deviceID, token string) error {
	_, err := saveConfig(map[string]string{
		"relay_url":    relay,
		"device_id":    deviceID,
		"device_token": token,
	})
	return err
}

// defaultDeviceName suggests a human-recognizable name so the claim page shows
// something better than "New device". The claimer can override it.
func defaultDeviceName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "Linux device"
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
