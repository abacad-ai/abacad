// Package enroll implements device-first self-enrollment: the client announces
// itself to a relay, receives its permanent id and token, and displays that id
// plus a rotating claim code until a human binds it to an account.
//
// It is deliberately free of X11, GTK, and daemon concerns so the daemon and the
// GUI can share one state machine and so the whole flow is unit-testable against
// an httptest server.
//
// The state machine is small:
//
//	no token?  ──► Register()               ──► persist id + token
//	     │
//	UNCLAIMED ──► Heartbeat() every N sec   ──► display id + claim code
//	     │  Claimed                ErrUnknownToken ──► wipe, re-register
//	CLAIMED  ───► dial wss://<relay>/device (the existing, unchanged path)
//
// The token survives the claim unchanged, so the transition from unclaimed to
// claimed needs no re-key and no second credential.
package enroll

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultRelay is the relay a client talks to when nothing else is configured.
// Self-hosting is a matter of pointing this elsewhere — see docs/trust.md on why
// a self-hosted relay with a real certificate needs no extra key material.
const DefaultRelay = "https://abacad.ai"

var (
	// ErrUnknownToken means the relay does not recognize our device token: the
	// registration was reaped, or the device was deleted or expired. The
	// credential is dead and the client must wipe it and register afresh.
	ErrUnknownToken = errors.New("relay does not recognize this device token")

	// ErrNotSupported means the relay predates self-enrollment. Callers fall back
	// to `abacad connect` rather than failing — a self-hoster who upgrades their
	// clients before their server should get a message, not a brick.
	ErrNotSupported = errors.New("relay does not support self-enrollment")
)

// Client talks to one relay's enrollment endpoints.
type Client struct {
	// Relay is the relay base URL, e.g. https://abacad.ai (no trailing slash).
	Relay string
	// HTTP is the transport; nil uses a client with a sane timeout.
	HTTP *http.Client
}

// New returns a Client for relay, falling back to DefaultRelay when blank.
func New(relay string) *Client {
	relay = strings.TrimRight(strings.TrimSpace(relay), "/")
	if relay == "" {
		relay = DefaultRelay
	}
	return &Client{Relay: relay, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Registration is what the relay issues to a brand-new client.
type Registration struct {
	DeviceID       string `json:"device_id"`
	DeviceToken    string `json:"device_token"` // shown once; persist it immediately
	ClaimCode      string `json:"claim_code"`
	ClaimExpiresIn int    `json:"claim_expires_in"`
	ClaimURL       string `json:"claim_url"`
	HeartbeatIn    int    `json:"heartbeat_in"`
}

// Status is one heartbeat's answer.
type Status struct {
	Claimed        bool   `json:"claimed"`
	DeviceID       string `json:"device_id"`
	ClaimCode      string `json:"claim_code"`       // unclaimed only
	ClaimExpiresIn int    `json:"claim_expires_in"` // unclaimed only
	HeartbeatIn    int    `json:"heartbeat_in"`
	Name           string `json:"name"`       // claimed only
	ClaimedBy      string `json:"claimed_by"` // claimed only — the account that claimed us
}

// Register announces this device to the relay and returns its permanent identity.
func (c *Client) Register(platform, name, ver string) (*Registration, error) {
	body, _ := json.Marshal(map[string]string{"platform": platform, "name": name, "version": ver})
	req, err := http.NewRequest("POST", c.Relay+"/api/devices/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, data, err := c.do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return nil, ErrNotSupported
	default:
		return nil, fmt.Errorf("register: server said %s: %s", resp.Status, serverError(data))
	}
	var r Registration
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.DeviceID == "" || r.DeviceToken == "" {
		return nil, errors.New("register: relay returned an incomplete registration")
	}
	return &r, nil
}

// Heartbeat proves liveness and reports whether we have been claimed. The same
// token works before and after the claim.
func (c *Client) Heartbeat(token string) (*Status, error) {
	req, err := http.NewRequest("POST", c.Relay+"/api/devices/self/heartbeat", nil)
	if err != nil {
		return nil, err
	}
	// Header only — this endpoint is new, so there is no legacy ?token= shape to
	// support and no reason to let the secret into a URL or a proxy log.
	req.Header.Set("Authorization", "Bearer "+token)

	resp, data, err := c.do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone, http.StatusUnauthorized:
		return nil, ErrUnknownToken
	default:
		return nil, fmt.Errorf("heartbeat: server said %s: %s", resp.Status, serverError(data))
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) do(req *http.Request) (*http.Response, []byte, error) {
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := h.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp, data, nil
}

// DeviceURL is the WebSocket URL this client dials once claimed. The relay's
// http(s) origin maps to the matching ws(s) scheme, so a self-hosted relay on
// plain http stays on plain ws for loopback development while a real deployment
// gets wss — matching the agent's own refusal to speak ws:// off-loopback.
func DeviceURL(relay string) (string, error) {
	relay = strings.TrimRight(strings.TrimSpace(relay), "/")
	if relay == "" {
		relay = DefaultRelay
	}
	u, err := url.Parse(relay)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
		// already a WebSocket origin
	default:
		return "", fmt.Errorf("unsupported relay scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/device"
	return u.String(), nil
}

// Interval converts a server-advertised second count into a poll interval,
// clamping nonsense values so a bad or missing hint can't spin the client.
func Interval(seconds int) time.Duration {
	d := time.Duration(seconds) * time.Second
	if d < 5*time.Second {
		return 20 * time.Second
	}
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

// serverError pulls the API's {"error": "..."} message out of a response body,
// falling back to the raw text.
func serverError(data []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		return e.Error
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "(no message)"
	}
	return s
}
