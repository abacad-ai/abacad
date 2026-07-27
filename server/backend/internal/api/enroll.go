package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"abacad/internal/activity"
	"abacad/internal/auth"
	"abacad/internal/store"
)

// Device self-enrollment: the device-first counterpart to pair.go.
//
// A freshly installed GUI client has nobody to belong to yet, so it registers
// itself (POST /api/devices/register), receives its permanent id and token, and
// displays that id plus a short-lived claim code on its OWN screen. A human with
// physical possession reads both off the device, previews it at /claim, and binds
// it to their account (POST /api/claim). The device never shows a URL and never
// asks the human to paste a secret into it.
//
// Two properties this design leans on, both deliberate:
//
//   - The id and the claim code are INDEPENDENT secrets and appear together only
//     on the device's screen. Knowing an id alone is useless; guessing a code
//     without the id is useless.
//   - An unclaimed device is not on the relay at all. It polls the heartbeat
//     below and only dials /device once claimed. So there is no code path by
//     which a command reaches a device nobody owns — that isolation is
//     structural (the row isn't in `devices` yet), not a policy check.
//
// The claim itself is held to exactly the same consent bar as pairApprove: an
// explicit `accepted: true`, recorded as activity.KindConsent with the SAME
// "enrollment.accepted" method string, so both enrollment routes appear
// identically in the audit trail.
const (
	// claimCodeTTL is how long one claim code lives. Short, because the code is
	// visible on a screen someone else may be able to see; long enough to survive
	// a signup and an OAuth round trip.
	claimCodeTTL = 5 * time.Minute

	// claimCodeRotateWithin triggers routine rotation on the heartbeat that finds
	// the current code this close to expiring, so the device always displays a
	// code with usable life left.
	claimCodeRotateWithin = 60 * time.Second

	// maxClaimAttempts is how many wrong codes one registration absorbs before the
	// code is force-rotated. Rotation both invalidates the attacker's guess space
	// and makes the attack visible: the code on the device's screen changes.
	maxClaimAttempts = 5

	// heartbeatInterval is the poll cadence advertised to unclaimed clients.
	heartbeatInterval = 20 * time.Second

	// registrationOnlineWithin is how recently an unclaimed registration must have
	// heartbeat to read as "online" in the claim preview. Generous relative to
	// heartbeatInterval so one dropped poll doesn't flap the indicator.
	registrationOnlineWithin = 45 * time.Second

	// maxLiveRegistrationsPerIP caps concurrent unclaimed registrations from one
	// address. Parking rows then costs a spammer a live heartbeating client each,
	// which is the point.
	maxLiveRegistrationsPerIP = 5

	// maxLiveRegistrations is a global soft cap. Past it, registration sheds load
	// rather than filling the disk; claims and existing devices are unaffected.
	maxLiveRegistrations = 50000
)

// registerDevice mints an unclaimed device identity for a client that has just
// been installed. Unauthenticated by necessity — the whole point is that no
// account exists yet.
//
// This is the one endpoint that lets an anonymous caller create a row and receive
// a bearer token, so it carries the heaviest throttling in the API. The token's
// entire capability is polling its own heartbeat until someone claims it, so the
// blast radius of over-issuance is storage, not access.
func (a *API) registerDevice(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if ok, retry := a.registrations.take(ip, time.Now()); !ok {
		writeThrottled(w, retry, "too many device registrations from this address")
		return
	}

	// Database-backed caps. Unlike the token bucket these survive a restart, so
	// they are what actually bound a patient attacker.
	if n, err := a.Store.LiveRegistrationsByIP(ip, time.Now().Add(-registrationOnlineWithin).Unix()); err == nil &&
		n >= maxLiveRegistrationsPerIP {
		writeThrottled(w, time.Minute, "too many unclaimed devices from this address — claim one first")
		return
	}
	if n, err := a.Store.CountRegistrations(); err == nil && n >= maxLiveRegistrations {
		writeErr(w, http.StatusServiceUnavailable, "device registration is temporarily unavailable")
		return
	}

	var body struct {
		Platform string `json:"platform"`
		Name     string `json:"name"`
		Version  string `json:"version"`
	}
	_ = decodeOptional(r, &body)

	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "New device"
	}
	reg, token, err := a.Store.CreateRegistration(
		strings.TrimSpace(body.Platform), name, strings.TrimSpace(body.Version), ip, claimCodeTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not register device")
		return
	}

	// No activity record here: activities are account-scoped and there is no
	// account yet. The trail starts at the claim, which is where a human takes
	// responsibility for the device.
	writeJSON(w, http.StatusCreated, map[string]any{
		"device_id":        reg.ID,
		"device_token":     token, // shown once
		"claim_code":       reg.ClaimCode,
		"claim_expires_in": int(time.Until(time.Unix(reg.ClaimExpiresAt, 0)).Seconds()),
		"claim_url":        httpURL(r, "/claim"),
		"heartbeat_in":     int(heartbeatInterval.Seconds()),
	})
}

// deviceHeartbeat is the unclaimed client's single loop: it proves liveness,
// receives the current claim code (rotated server-side as needed), and learns the
// moment it has been claimed. Authenticated by the device token the client
// already holds.
//
// The token outlives the claim unchanged, so this resolves against registrations
// first and then real devices — the client's credential store never changes and
// the unclaimed/claimed transition needs no re-key.
func (a *API) deviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	tokenHash, ok := deviceTokenHash(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing device token")
		return
	}

	reg, err := a.Store.RegistrationByTokenHash(tokenHash)
	if err == nil {
		a.heartbeatUnclaimed(w, reg)
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}

	// Not an unclaimed registration — is it a claimed device?
	d, err := a.Store.DeviceByTokenHash(tokenHash)
	if errors.Is(err, store.ErrNotFound) {
		// Registration was reaped, or the device was deleted/expired. Either way
		// this credential is dead; the client wipes it and registers afresh.
		writeErr(w, http.StatusNotFound, "unknown device token")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}

	// Disclose WHO claimed it. This is the mitigation for the one new attack this
	// flow creates: someone who read the id and code off the screen can claim the
	// device to their own account, and without this the victim would never know.
	// The client shows this alongside a "that wasn't me — disconnect" action.
	var claimedBy string
	if acc, err := a.Store.AccountByID(d.AccountID); err == nil {
		claimedBy = acc.Email
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"claimed":    true,
		"device_id":  d.ID,
		"name":       d.Name,
		"claimed_by": claimedBy,
	})
}

// heartbeatUnclaimed answers a still-unclaimed device: bump liveness, rotate the
// code if it is close to expiring, and hand back whatever the device should now
// display. The device never asks for rotation — it renders what it is given.
func (a *API) heartbeatUnclaimed(w http.ResponseWriter, reg store.Registration) {
	_ = a.Store.TouchRegistration(reg.ID)

	if time.Until(time.Unix(reg.ClaimExpiresAt, 0)) < claimCodeRotateWithin {
		if rotated, err := a.Store.RotateClaimCode(reg.ID, claimCodeTTL); err == nil {
			reg = rotated
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"claimed":          false,
		"device_id":        reg.ID,
		"claim_code":       reg.ClaimCode,
		"claim_expires_in": int(time.Until(time.Unix(reg.ClaimExpiresAt, 0)).Seconds()),
		"heartbeat_in":     int(heartbeatInterval.Seconds()),
	})
}

// claimPreview lets a visitor confirm WHICH device they are about to claim before
// being asked to create an account. Public: holding the id and the code is the
// proof, and both are only readable off the device itself.
//
// It deliberately does NOT consume the code — the human still has to sign up or
// log in (possibly through an OAuth redirect) and come back.
//
// Every failure mode returns an identical 404 so this cannot be used to test
// whether a device id exists. That matters more here than in most places: ids are
// the addressing scheme for the whole product, and RustDesk's equivalent endpoint
// distinguishing "no such id" from "offline" is exactly the enumeration oracle
// worth not reproducing.
func (a *API) claimPreview(w http.ResponseWriter, r *http.Request) {
	if ok, retry := a.claimAttempts.take(clientIP(r), time.Now()); !ok {
		writeThrottled(w, retry, "too many attempts")
		return
	}
	var body struct {
		DeviceID  string `json:"device_id"`
		ClaimCode string `json:"claim_code"`
	}
	if !decode(w, r, &body) {
		return
	}
	reg, ok := a.lookupClaimable(w, body.DeviceID, body.ClaimCode)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":     reg.ID,
		"name":          reg.Name,
		"platform":      reg.Platform,
		"version":       reg.Version,
		"online":        reg.LastSeen > time.Now().Add(-registrationOnlineWithin).Unix(),
		"registered_at": time.Unix(reg.CreatedAt, 0).UTC().Format(time.RFC3339),
	})
}

// claimDevice binds an unclaimed device to the signed-in account. Session-gated:
// the approving account becomes the owner, exactly as in pairApprove.
func (a *API) claimDevice(w http.ResponseWriter, r *http.Request) {
	acc := account(r)
	if ok, retry := a.claimAttempts.take("acct:"+acc.ID, time.Now()); !ok {
		writeThrottled(w, retry, "too many attempts")
		return
	}
	var body struct {
		DeviceID  string `json:"device_id"`
		ClaimCode string `json:"claim_code"`
		Name      string `json:"name"`
		// Accepted must be true: the operator acknowledges that claiming lets an
		// agent see, control, and transfer files on the device, and confirms they
		// are authorized to operate it. Same gate as pairApprove — a device
		// arriving by this route must clear the identical bar.
		Accepted *bool `json:"accepted"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Accepted == nil || !*body.Accepted {
		writeErr(w, http.StatusUnprocessableEntity, "approval requires acknowledging what pairing authorizes")
		return
	}
	if _, ok := a.lookupClaimable(w, body.DeviceID, body.ClaimCode); !ok {
		return
	}

	name := strings.TrimSpace(body.Name)
	// enrollmentExpiry is evaluated HERE, at claim time, not at registration:
	// the 24h enrollment TTL is measured from when an account took responsibility
	// for the device, not from when a binary was installed. Do not move it.
	d, err := a.Store.ClaimRegistration(
		strings.TrimSpace(body.DeviceID), normalizeUserCode(body.ClaimCode), acc.ID, name, a.enrollmentExpiry())
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found") // raced, or code just rotated
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not claim device")
		return
	}

	a.record(acc.ID, store.Activity{Kind: activity.KindConsent, Method: "enrollment.accepted", Detail: d.Name})
	a.record(acc.ID, store.Activity{Kind: activity.KindDeviceCreate, DeviceID: d.ID, Detail: d.Name})
	writeJSON(w, http.StatusOK, map[string]any{"device_id": d.ID, "name": d.Name})
}

// lookupClaimable resolves an id+code pair to a claimable registration, writing
// the response and returning false on any failure.
//
// All failures look identical from outside (404 "device not found"). Inside, a
// wrong code against a REAL id is counted, and past maxClaimAttempts the code is
// force-rotated — bounding an online guessing attack and surfacing it on the
// device's screen.
func (a *API) lookupClaimable(w http.ResponseWriter, deviceID, claimCode string) (store.Registration, bool) {
	const notFound = "device not found"

	id := strings.ToLower(strings.TrimSpace(deviceID))
	code := normalizeUserCode(claimCode)
	if id == "" || code == "" {
		writeErr(w, http.StatusNotFound, notFound)
		return store.Registration{}, false
	}
	reg, err := a.Store.RegistrationByID(id)
	if err != nil {
		// Unknown id, or already claimed (the row is gone once graduated).
		writeErr(w, http.StatusNotFound, notFound)
		return store.Registration{}, false
	}
	if reg.ClaimCode != code || reg.ClaimExpiresAt <= time.Now().Unix() {
		if n, err := a.Store.NoteClaimAttempt(reg.ID); err == nil && n >= maxClaimAttempts {
			_, _ = a.Store.RotateClaimCode(reg.ID, claimCodeTTL)
		}
		writeErr(w, http.StatusNotFound, notFound)
		return store.Registration{}, false
	}
	return reg, true
}

// deviceTokenHash reads the device bearer token from the request and returns its
// hash. Header-only: unlike /device (which still accepts ?token= for older
// clients), these endpoints are new, so there is no legacy shape to support and
// no reason to let the secret into a URL.
func deviceTokenHash(r *http.Request) (string, bool) {
	token := auth.BearerToken(r)
	if token == "" {
		return "", false
	}
	return auth.HashToken(token), true
}
