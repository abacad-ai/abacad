package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

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

// enrollRelay resolves which relay to enroll with, in precedence order.
func enrollRelay(cfg map[string]string, relayFlag string) string {
	return firstNonEmpty(relayFlag, os.Getenv("ABACAD_RELAY_URL"), cfg["relay_url"], enroll.DefaultRelay)
}

// newSession builds the shared enrollment state machine, wired to this process's
// config file and terminal. internal/gui gets the same object, with its own
// callbacks, so both front-ends drive identical logic.
func newSession(relay string) *enroll.Session {
	return &enroll.Session{
		Relay:    relay,
		Platform: detectPlatform(),
		Name:     defaultDeviceName(),
		Version:  version.Version,
		Persist:  persist,
		OnCode: func(deviceID, code string) {
			printClaimPrompt(deviceID, code, relay)
		},
		OnStatus: func(msg string) { log.Print(msg) },
	}
}

// ensureEnrolled resolves this machine's device credentials, self-registering
// with the relay and blocking until a human claims it if necessary.
//
// It returns nil (with no error) when self-enrollment does not apply — headless,
// or a relay too old to support it — so the caller can fall back to the legacy
// configured-URL path and print the appropriate guidance.
//
// allowRegister is true here: reaching this function means someone typed
// `abacad` at a shell and is watching the claim box print to their terminal.
// That is the explicit intent the GUI has to ask for with a button, which is why
// the GUI does not use this path.
func ensureEnrolled(ctx context.Context, cfg map[string]string, relayFlag string) (*enrolled, error) {
	if detectPlatform() == "linux-headless" {
		return nil, nil
	}
	relay := enrollRelay(cfg, relayFlag)

	res, err := newSession(relay).Run(ctx, cfg["device_token"], cfg["device_id"], true)
	switch {
	case errors.Is(err, enroll.ErrNotSupported):
		log.Printf("relay %s does not support self-enrollment — falling back to `abacad connect`", relay)
		return nil, nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return nil, err
	case err != nil:
		return nil, err
	}
	return &enrolled{deviceURL: res.DeviceURL, token: res.Token, deviceID: res.DeviceID}, nil
}

// printClaimPrompt is the terminal equivalent of the GUI's setup screen: the two
// things a human needs to read off this machine, and where to type them.
func printClaimPrompt(deviceID, claimCode, relay string) {
	id := deviceID
	base := strings.TrimRight(relay, "/")
	fmt.Fprintf(os.Stderr, `
  ┌─ Add this device ─────────────────────────────
  │
  │   Device ID    %s
  │   Claim code   %s
  │
  │   Open %s/claim
  │   and enter both to add this device to your account.
  │
  │   The code changes every few minutes; anyone who can
  │   read both lines can claim this device.
  │
  │   Relay: %s
%s  └───────────────────────────────────────────────

`, id, claimCode, base, base, selfHostHint(relay))
}

// selfHostHint nudges toward a self-hosted relay, but only when the device is
// actually on the default one — telling someone already running their own relay
// to run their own relay is noise. Framed as latency and control, not security:
// a relay behind a home NAT can be *worse* reachable than the hosted one, so
// "better connectivity" would be an overclaim.
func selfHostHint(relay string) string {
	if strings.TrimRight(strings.TrimSpace(relay), "/") != enroll.DefaultRelay {
		return ""
	}
	return "  │   On the same network as your agent? Your own relay is\n" +
		"  │   lower latency — abacad.ai/docs/guides/self-hosting/\n"
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
