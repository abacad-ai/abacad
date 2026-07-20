// Package api serves the dashboard's JSON endpoints under /api, authenticated by
// the session cookie (except register/login). Secrets (device/MCP tokens) are
// returned exactly once, on create/rotate; reads never expose them.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"abacad/internal/activity"
	"abacad/internal/auth"
	"abacad/internal/events"
	"abacad/internal/protocol"
	"abacad/internal/relay"
	"abacad/internal/screenshot"
	"abacad/internal/sshjump"
	"abacad/internal/store"
)

const sessionTTL = 30 * 24 * time.Hour

// API holds dependencies for the dashboard endpoints.
type API struct {
	Store      *store.Store
	Hub        *relay.Hub
	Events     *events.Log        // per-device live activity ring
	Activity   *activity.Recorder // persistent account trail (Activities page)
	Shots      *screenshot.Store  // per-device last-screenshot cache
	BaseDomain string             // domain devices are addressed under, for the ssh_host hint

	logins *loginLimiter // per-IP login throttle; initialized in Handler
}

type ctxKey int

const accountKey ctxKey = 0

// Handler returns the /api router (to be mounted at /api/).
func (a *API) Handler() http.Handler {
	if a.logins == nil {
		a.logins = newLoginLimiter()
	}
	mux := http.NewServeMux()

	// Public auth endpoints.
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)

	// Authenticated endpoints.
	mux.Handle("GET /api/auth/me", a.auth(a.me))
	mux.Handle("GET /api/devices", a.auth(a.listDevices))
	mux.Handle("POST /api/devices", a.auth(a.createDevice))
	mux.Handle("GET /api/devices/{id}", a.auth(a.getDevice))
	mux.Handle("PATCH /api/devices/{id}", a.auth(a.renameDevice))
	mux.Handle("DELETE /api/devices/{id}", a.auth(a.deleteDevice))
	mux.Handle("POST /api/devices/{id}/rotate-token", a.auth(a.rotateDeviceToken))
	mux.Handle("GET /api/devices/{id}/screenshot", a.auth(a.deviceScreenshot))
	mux.Handle("GET /api/devices/{id}/events", a.auth(a.deviceEvents))
	mux.Handle("GET /api/mcp-token", a.auth(a.getMCPToken))
	mux.Handle("POST /api/mcp-token/rotate", a.auth(a.rotateMCPToken))
	mux.Handle("GET /api/activities", a.auth(a.listActivities))

	// SSH keys authorize the jump host (ssh <device>.<base-domain>).
	mux.Handle("GET /api/ssh-keys", a.auth(a.listSSHKeys))
	mux.Handle("POST /api/ssh-keys", a.auth(a.addSSHKey))
	mux.Handle("DELETE /api/ssh-keys/{id}", a.auth(a.deleteSSHKey))

	return mux
}

// --- auth middleware ---

func (a *API) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acc, err := a.Store.AccountBySession(auth.SessionID(r))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not signed in")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), accountKey, acc)))
	})
}

func account(r *http.Request) store.Account {
	acc, _ := r.Context().Value(accountKey).(store.Account)
	return acc
}

// --- auth handlers ---

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	c.Email = strings.TrimSpace(strings.ToLower(c.Email))
	if !strings.Contains(c.Email, "@") || len(c.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "valid email and a password of at least 6 characters are required")
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	acc, err := a.Store.CreateAccount(c.Email, hash)
	if errors.Is(err, store.ErrEmailTaken) {
		writeErr(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create account")
		return
	}
	a.startSession(w, r, acc)
	a.record(acc.ID, store.Activity{Kind: activity.KindRegister, Detail: acc.Email})
	writeJSON(w, http.StatusCreated, map[string]string{"account_id": acc.ID, "email": acc.Email})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	ip := clientIP(r)
	if ok, retry := a.logins.allowed(ip, time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "too many login attempts; try again later")
		return
	}
	c.Email = strings.TrimSpace(strings.ToLower(c.Email))
	acc, err := a.Store.AccountByEmail(c.Email)
	if err != nil || !auth.CheckPassword(acc.PasswordHash, c.Password) {
		a.logins.recordFail(ip, time.Now())
		// A wrong password on a real account is worth the trail; an unknown email
		// has no account to attach it to.
		if err == nil {
			a.record(acc.ID, store.Activity{Kind: activity.KindLoginFailed, Outcome: "failed", Detail: "wrong password"})
		}
		writeErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	a.logins.reset(ip)
	a.startSession(w, r, acc)
	a.record(acc.ID, store.Activity{Kind: activity.KindLogin})
	writeJSON(w, http.StatusOK, map[string]string{"account_id": acc.ID, "email": acc.Email})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if sid := auth.SessionID(r); sid != "" {
		if acc, err := a.Store.AccountBySession(sid); err == nil {
			a.record(acc.ID, store.Activity{Kind: activity.KindLogout})
		}
		_ = a.Store.DeleteSession(sid)
	}
	http.SetCookie(w, a.clearCookie(r))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	acc := account(r)
	writeJSON(w, http.StatusOK, map[string]string{"account_id": acc.ID, "email": acc.Email})
}

// --- device handlers ---

type deviceView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Online       bool   `json:"online"`
	Platform     string `json:"platform,omitempty"` // e.g. "android", "macos"; blank if unset
	LastSeen     string `json:"last_seen,omitempty"`
	CreatedAt    string `json:"created_at"`
	SSHHost      string `json:"ssh_host,omitempty"`      // ssh <ssh_host> reaches this device via the jump
	ScreenshotAt int64  `json:"screenshot_at,omitempty"` // unix seconds of the last stored screenshot; 0/absent if none
}

func (a *API) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := a.Store.DevicesByAccount(account(r).ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list devices")
		return
	}
	out := make([]deviceView, 0, len(devices))
	for _, d := range devices {
		out = append(out, a.viewDevice(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// getDevice returns a single device the caller owns — the detail page loads it
// directly by id so a deep link (or hard refresh) works without the full list.
func (a *API) getDevice(w http.ResponseWriter, r *http.Request) {
	d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load device")
		return
	}
	writeJSON(w, http.StatusOK, a.viewDevice(d))
}

func (a *API) createDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}
	_ = decodeOptional(r, &body)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "New device"
	}
	d, token, err := a.Store.CreateDevice(account(r).ID, name, strings.TrimSpace(body.Platform))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create device")
		return
	}
	a.record(account(r).ID, store.Activity{Kind: activity.KindDeviceCreate, DeviceID: d.ID, Detail: d.Name})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           d.ID,
		"name":         d.Name,
		"platform":     d.Platform,
		"device_token": token, // shown once
		"wss_url":      wsURL(r, token),
	})
}

func (a *API) renameDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	err := a.Store.RenameDevice(r.PathValue("id"), account(r).ID, strings.TrimSpace(body.Name))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not rename device")
		return
	}
	a.record(account(r).ID, store.Activity{Kind: activity.KindDeviceRename, DeviceID: r.PathValue("id"), Detail: strings.TrimSpace(body.Name)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) deleteDevice(w http.ResponseWriter, r *http.Request) {
	// Load the device first so its name survives in the trail after the row is gone.
	name := ""
	if d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID); err == nil {
		name = d.Name
	}
	err := a.Store.DeleteDevice(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete device")
		return
	}
	if a.Shots != nil {
		a.Shots.Delete(r.PathValue("id"))
	}
	a.record(account(r).ID, store.Activity{Kind: activity.KindDeviceDelete, DeviceID: r.PathValue("id"), Detail: name})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) rotateDeviceToken(w http.ResponseWriter, r *http.Request) {
	token, err := a.Store.RotateDeviceToken(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not rotate token")
		return
	}
	a.record(account(r).ID, store.Activity{Kind: activity.KindDeviceToken, DeviceID: r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]string{"device_token": token, "wss_url": wsURL(r, token)})
}

// deviceScreenshot serves a device's screen as a JPEG for an <img> src. It has
// two modes:
//
//   - default: serve the last stored screenshot (fast, no device round-trip).
//     Works whether the device is online or offline, so the dashboard can show a
//     device's screen instantly on load and keep showing it after it drops.
//   - ?live=1: capture a fresh frame from the connected device, store it as the
//     new last screenshot, and serve it. This is the dashboard's live poll; on a
//     capture failure it falls back to the last stored frame.
func (a *API) deviceScreenshot(w http.ResponseWriter, r *http.Request) {
	d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load device")
		return
	}

	if r.URL.Query().Get("live") == "1" {
		if jpeg, ok := a.captureLive(r, d.ID); ok {
			_ = a.Shots.Save(d.ID, jpeg)
			writeJPEG(w, jpeg)
			return
		}
		// Capture failed (offline, timeout, malformed): fall through to the last
		// stored frame so the live poll keeps showing something instead of breaking.
	}

	f, modTime, err := a.Shots.Open(d.ID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no screenshot available")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", modTime, f)
}

// captureLive asks the connected device for a fresh JPEG (no UI tree — the
// dashboard only needs the image). Returns false if the device is offline or the
// capture fails; the caller falls back to the last stored frame.
func (a *API) captureLive(r *http.Request, deviceID string) ([]byte, bool) {
	dc, ok := a.Hub.Get(deviceID)
	if !ok {
		return nil, false
	}
	// Tag this as a dashboard-originated command so the activity log can tell it
	// apart from the agent's own screenshots.
	ctx := relay.WithSource(r.Context(), "dashboard")
	raw, err := dc.Send(ctx, protocol.MethodScreenshot, map[string]any{"include_ui_tree": false}, 0)
	if err != nil {
		return nil, false
	}
	var res protocol.ScreenshotResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, false
	}
	jpeg, err := base64.StdEncoding.DecodeString(res.PNGBase64)
	if err != nil || len(jpeg) == 0 {
		return nil, false
	}
	return jpeg, true
}

func writeJPEG(w http.ResponseWriter, jpeg []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jpeg)
}

// deviceEvents returns the device's recent activity log (connects, disconnects
// with reason, and per-command timing/outcome) so the dashboard can show what's
// happening — and why a call timed out or the device dropped.
func (a *API) deviceEvents(w http.ResponseWriter, r *http.Request) {
	d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load device")
		return
	}
	var evs []events.Event
	if a.Events != nil {
		evs = a.Events.Recent(d.ID, 0) // newest first
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"online": a.Hub.Online(d.ID),
		"events": evs,
	})
}

// --- MCP token handlers ---
func (a *API) getMCPToken(w http.ResponseWriter, r *http.Request) {
	info, err := a.Store.MCPToken(account(r).ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read token")
		return
	}
	resp := map[string]any{"exists": info.Exists}
	if info.Exists {
		resp["created_at"] = time.Unix(info.CreatedAt, 0).UTC().Format(time.RFC3339)
		if info.LastUsed > 0 {
			resp["last_used"] = time.Unix(info.LastUsed, 0).UTC().Format(time.RFC3339)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) rotateMCPToken(w http.ResponseWriter, r *http.Request) {
	token, err := a.Store.RotateMCPToken(account(r).ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not rotate token")
		return
	}
	a.record(account(r).ID, store.Activity{Kind: activity.KindMCPToken})
	writeJSON(w, http.StatusOK, map[string]string{"mcp_token": token, "mcp_url": httpURL(r, "/mcp")})
}

// --- SSH key handlers ---

type sshKeyView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
	CreatedAt   string `json:"created_at"`
	LastUsed    string `json:"last_used,omitempty"`
}

func (a *API) listSSHKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.Store.SSHKeysByAccount(account(r).ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list ssh keys")
		return
	}
	out := make([]sshKeyView, 0, len(keys))
	for _, k := range keys {
		v := sshKeyView{
			ID: k.ID, Name: k.Name, Fingerprint: k.Fingerprint, PublicKey: k.PublicKey,
			CreatedAt: time.Unix(k.CreatedAt, 0).UTC().Format(time.RFC3339),
		}
		if k.LastUsed > 0 {
			v.LastUsed = time.Unix(k.LastUsed, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) addSSHKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if !decode(w, r, &body) {
		return
	}
	// Parse the pasted authorized_keys line; derive a canonical fingerprint and a
	// normalized single-line form to store.
	pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(body.PublicKey)))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not parse public key (paste an OpenSSH authorized_keys line)")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = comment // fall back to the key's trailing comment (often user@host)
	}
	normalized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	k, err := a.Store.AddSSHKey(account(r).ID, name, ssh.FingerprintSHA256(pub), normalized)
	if errors.Is(err, store.ErrKeyExists) {
		writeErr(w, http.StatusConflict, "that key is already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add ssh key")
		return
	}
	a.record(account(r).ID, store.Activity{Kind: activity.KindSSHKeyAdd, Detail: k.Name + " " + k.Fingerprint})
	writeJSON(w, http.StatusCreated, sshKeyView{
		ID: k.ID, Name: k.Name, Fingerprint: k.Fingerprint, PublicKey: k.PublicKey,
		CreatedAt: time.Unix(k.CreatedAt, 0).UTC().Format(time.RFC3339),
	})
}

func (a *API) deleteSSHKey(w http.ResponseWriter, r *http.Request) {
	// Capture the key's label before it's gone (accounts hold a handful of keys).
	detail := ""
	if keys, err := a.Store.SSHKeysByAccount(account(r).ID); err == nil {
		for _, k := range keys {
			if k.ID == r.PathValue("id") {
				detail = k.Name + " " + k.Fingerprint
			}
		}
	}
	err := a.Store.DeleteSSHKey(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "ssh key not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete ssh key")
		return
	}
	a.record(account(r).ID, store.Activity{Kind: activity.KindSSHKeyRemove, Detail: detail})
	w.WriteHeader(http.StatusNoContent)
}

// --- session cookie ---

func (a *API) startSession(w http.ResponseWriter, r *http.Request, acc store.Account) {
	sid, err := a.Store.CreateSession(acc.ID, r.UserAgent(), sessionTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		Expires:  time.Now().Add(sessionTTL),
	})
}

func (a *API) clearCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name: auth.SessionCookie, Value: "", Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r),
		MaxAge: -1,
	}
}

func (a *API) viewDevice(d store.Device) deviceView {
	v := deviceView{
		ID:        d.ID,
		Name:      d.Name,
		Online:    a.Hub.Online(d.ID),
		Platform:  d.Platform,
		CreatedAt: time.Unix(d.CreatedAt, 0).UTC().Format(time.RFC3339),
	}
	if d.LastSeen > 0 {
		v.LastSeen = time.Unix(d.LastSeen, 0).UTC().Format(time.RFC3339)
	}
	if a.BaseDomain != "" {
		v.SSHHost = sshjump.HostForDevice(d.ID, a.BaseDomain)
	}
	if a.Shots != nil {
		if at, ok := a.Shots.At(d.ID); ok {
			v.ScreenshotAt = at
		}
	}
	return v
}

// --- request/response helpers ---

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// decodeOptional decodes a possibly-empty body without erroring on EOF.
func decodeOptional(r *http.Request, v any) error {
	err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// isHTTPS reports whether the original client request was over TLS (directly or
// via a reverse proxy).
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func publicHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return h
	}
	return r.Host
}

// wsURL builds the device connection URL (wss when the site is https).
func wsURL(r *http.Request, token string) string {
	scheme := "ws"
	if isHTTPS(r) {
		scheme = "wss"
	}
	return scheme + "://" + publicHost(r) + "/device?token=" + token
}

// httpURL builds an absolute URL to a path on this server.
func httpURL(r *http.Request, path string) string {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + publicHost(r) + path
}
