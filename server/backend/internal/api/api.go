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
	"strings"
	"time"

	"abacad/internal/auth"
	"abacad/internal/events"
	"abacad/internal/protocol"
	"abacad/internal/relay"
	"abacad/internal/store"
)

const sessionTTL = 30 * 24 * time.Hour

// API holds dependencies for the dashboard endpoints.
type API struct {
	Store  *store.Store
	Hub    *relay.Hub
	Events *events.Log // per-device activity log
}

type ctxKey int

const accountKey ctxKey = 0

// Handler returns the /api router (to be mounted at /api/).
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public auth endpoints.
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)

	// Authenticated endpoints.
	mux.Handle("GET /api/auth/me", a.auth(a.me))
	mux.Handle("GET /api/devices", a.auth(a.listDevices))
	mux.Handle("POST /api/devices", a.auth(a.createDevice))
	mux.Handle("PATCH /api/devices/{id}", a.auth(a.renameDevice))
	mux.Handle("DELETE /api/devices/{id}", a.auth(a.deleteDevice))
	mux.Handle("POST /api/devices/{id}/rotate-token", a.auth(a.rotateDeviceToken))
	mux.Handle("GET /api/devices/{id}/screenshot", a.auth(a.deviceScreenshot))
	mux.Handle("GET /api/devices/{id}/events", a.auth(a.deviceEvents))
	mux.Handle("GET /api/mcp-token", a.auth(a.getMCPToken))
	mux.Handle("POST /api/mcp-token/rotate", a.auth(a.rotateMCPToken))

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
	writeJSON(w, http.StatusCreated, map[string]string{"account_id": acc.ID, "email": acc.Email})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	c.Email = strings.TrimSpace(strings.ToLower(c.Email))
	acc, err := a.Store.AccountByEmail(c.Email)
	if err != nil || !auth.CheckPassword(acc.PasswordHash, c.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	a.startSession(w, r, acc)
	writeJSON(w, http.StatusOK, map[string]string{"account_id": acc.ID, "email": acc.Email})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if sid := auth.SessionID(r); sid != "" {
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	Online    bool   `json:"online"`
	LastSeen  string `json:"last_seen,omitempty"`
	CreatedAt string `json:"created_at"`
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

func (a *API) createDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = decodeOptional(r, &body)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "New device"
	}
	d, token, err := a.Store.CreateDevice(account(r).ID, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create device")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           d.ID,
		"name":         d.Name,
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
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) deleteDevice(w http.ResponseWriter, r *http.Request) {
	err := a.Store.DeleteDevice(r.PathValue("id"), account(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete device")
		return
	}
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
	writeJSON(w, http.StatusOK, map[string]string{"device_token": token, "wss_url": wsURL(r, token)})
}

// deviceScreenshot proxies a live screenshot from the device: it asks the
// connected device for a JPEG (no UI tree — the dashboard only needs the image)
// and streams the decoded bytes back so the frontend can use it as an <img> src.
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
	dc, ok := a.Hub.Get(d.ID)
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "device offline")
		return
	}
	// Tag this as a dashboard-originated command so the activity log can tell it
	// apart from the agent's own screenshots.
	ctx := relay.WithSource(r.Context(), "dashboard")
	raw, err := dc.Send(ctx, protocol.MethodScreenshot, map[string]any{"include_ui_tree": false}, 0)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	var res protocol.ScreenshotResult
	if err := json.Unmarshal(raw, &res); err != nil {
		writeErr(w, http.StatusBadGateway, "device returned a malformed screenshot")
		return
	}
	png, err := base64.StdEncoding.DecodeString(res.PNGBase64)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "device returned a malformed screenshot")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
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
	writeJSON(w, http.StatusOK, map[string]string{"mcp_token": token, "mcp_url": httpURL(r, "/mcp")})
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
		CreatedAt: time.Unix(d.CreatedAt, 0).UTC().Format(time.RFC3339),
	}
	if d.LastSeen > 0 {
		v.LastSeen = time.Unix(d.LastSeen, 0).UTC().Format(time.RFC3339)
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
