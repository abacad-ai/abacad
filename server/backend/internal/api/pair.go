package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"abacad/internal/activity"
	"abacad/internal/store"
)

// Device-authorization pairing (RFC 8628): the CLI-native alternative to copying
// a device token out of the dashboard. `abacad connect` calls /pair/start to get
// a short user_code, the human approves it at /pair in a logged-in browser
// (POST /api/devices/pair), and the CLI's /pair/poll then receives the freshly
// minted device token. Storage + token minting live in store.device_pairings.
const (
	pairingTTL      = 10 * time.Minute
	pairPollSeconds = 3 // client poll-interval hint (seconds)
)

// pairStart opens a pending pairing and returns the codes. Public: the CLI has no
// session yet, so binding to an account happens later, at approval. The CLI
// reports its own platform so the approval page can show what it's authorizing.
func (a *API) pairStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Platform string `json:"platform"`
	}
	_ = decodeOptional(r, &body)
	deviceCode, userCode, err := a.Store.CreatePairing(strings.TrimSpace(body.Platform), pairingTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start pairing")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"device_code":               deviceCode,
		"user_code":                 userCode,
		"verification_uri":          httpURL(r, "/pair"),
		"verification_uri_complete": httpURL(r, "/pair?code="+userCode),
		"interval":                  pairPollSeconds,
		"expires_in":                int(pairingTTL.Seconds()),
	})
}

// pairPoll is the CLI's wait loop. Public, addressed by the secret device_code.
// Pending -> 202; approved -> mint the device and return the token once;
// expired/used/denied -> a terminal 4xx so the CLI stops polling.
func (a *API) pairPoll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceCode string `json:"device_code"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := a.Store.PairingByDeviceCode(strings.TrimSpace(body.DeviceCode))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "unknown pairing")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "pairing lookup failed")
		return
	}

	switch {
	case p.Consumed:
		writeErr(w, http.StatusGone, "pairing already used")
	case time.Now().Unix() > p.ExpiresAt:
		writeErr(w, http.StatusGone, "pairing expired")
	case p.Status == store.PairingDenied:
		writeErr(w, http.StatusForbidden, "pairing denied")
	case p.Status == store.PairingApproved:
		d, token, err := a.Store.ConsumePairing(p.DeviceCode)
		if errors.Is(err, store.ErrNotFound) {
			// Lost a race to a concurrent poll that already consumed it.
			writeErr(w, http.StatusGone, "pairing already used")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not complete pairing")
			return
		}
		a.record(d.AccountID, store.Activity{Kind: activity.KindDeviceCreate, DeviceID: d.ID, Detail: d.Name})
		writeJSON(w, http.StatusOK, map[string]string{
			"device_token": token, // shown once
			"wss_url":      wsURL(r, token),
		})
	default: // pending
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "pending", "interval": pairPollSeconds})
	}
}

// pairLookup lets the approval page confirm a code exists and show what it will
// authorize (the CLI-reported platform) before the human commits. Session-gated,
// but reveals nothing an approver couldn't already act on: to matter, the caller
// must already hold the short-lived code, and approval binds the device to the
// caller's own account. Only pending pairings are surfaced.
func (a *API) pairLookup(w http.ResponseWriter, r *http.Request) {
	code := normalizeUserCode(r.URL.Query().Get("code"))
	if code == "" {
		writeErr(w, http.StatusBadRequest, "invalid code")
		return
	}
	p, err := a.Store.PairingByUserCode(code)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "code not found or expired")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "pairing lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_code": p.UserCode,
		"platform":  p.Platform,
		"status":    p.Status,
	})
}

// pairApprove is the human side, invoked from the /pair page. Session-gated: the
// approving account (from the cookie) becomes the device's owner.
func (a *API) pairApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserCode string `json:"user_code"`
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}
	if !decode(w, r, &body) {
		return
	}
	code := normalizeUserCode(body.UserCode)
	if code == "" {
		writeErr(w, http.StatusBadRequest, "invalid code")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "New device"
	}
	err := a.Store.ApprovePairing(code, account(r).ID, name, strings.TrimSpace(body.Platform))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "code not found, expired, or already used")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not approve pairing")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// normalizeUserCode is tolerant of how a human retypes a code — case, spaces, and
// the group dash are all optional — and re-emits the canonical "XXXX-XXXX" form
// that matches what CreatePairing stored. Wrong length -> "" (invalid).
func normalizeUserCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	c := b.String()
	if len(c) != 8 {
		return ""
	}
	return c[:4] + "-" + c[4:]
}
