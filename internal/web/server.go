package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"abacad/internal/config"
	"abacad/internal/discovery"
	"abacad/internal/gateway"
	"abacad/internal/inventory"
	mcpserver "abacad/internal/mcp"
)

//go:embed all:dist all:assets/novnc
var assets embed.FS

type Server struct {
	inv     *inventory.Inventory
	gw      *gateway.Gateway
	disc    *discovery.Discovery
	mcp     *mcpserver.Server
	log     *slog.Logger
	version string
	dist    fs.FS
}

func New(inv *inventory.Inventory, gw *gateway.Gateway, disc *discovery.Discovery, mcp *mcpserver.Server, log *slog.Logger, version string) *Server {
	return &Server{inv: inv, gw: gw, disc: disc, mcp: mcp, log: log, version: version}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/session", s.session)

	if s.mcp != nil {
		mux.Handle("/mcp", s.mcp.HTTPHandler())
	}

	mux.HandleFunc("GET /api/devices", s.auth(s.listDevices))
	mux.HandleFunc("POST /api/devices", s.auth(s.addDevice))
	mux.HandleFunc("PATCH /api/devices/{id}", s.auth(s.updateDevice))
	mux.HandleFunc("DELETE /api/devices/{id}", s.auth(s.deleteDevice))
	mux.HandleFunc("POST /api/devices/{id}/probe", s.auth(s.probeDevice))
	mux.HandleFunc("GET /api/discovered", s.auth(s.discovered))

	mux.HandleFunc("GET /vnc/{id}", s.view)
	mux.HandleFunc("GET /ws/vnc/{id}", s.websocket)

	distFS, err := fs.Sub(assets, "dist")
	if err != nil {
		panic("embedded frontend: " + err.Error())
	}
	s.dist = distFS
	novncFS, err := fs.Sub(assets, "assets/novnc")
	if err != nil {
		panic("embedded noVNC: " + err.Error())
	}

	// Vite emits hashed files under dist/assets; they can be cached forever.
	distAssets, err := fs.Sub(distFS, "assets")
	if err == nil {
		mux.Handle("GET /assets/", http.StripPrefix("/assets/", immutable(http.FileServer(http.FS(distAssets)))))
	}
	mux.HandleFunc("GET /novnc/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/novnc/vnc.html", http.StatusFound)
	})
	mux.Handle("GET /novnc/", http.StripPrefix("/novnc/", noCache(http.FileServer(http.FS(novncFS)))))
	mux.HandleFunc("GET /novnc", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/novnc/vnc.html", http.StatusFound)
	})
	mux.HandleFunc("GET /favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(distFS, "favicon.svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(b)
	})
	return securityHeaders(mux)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	s.serveIndex(w)
}

func (s *Server) view(w http.ResponseWriter, r *http.Request) {
	if !s.loggedIn(r) {
		http.Redirect(w, r, "/?next="+r.URL.Path, http.StatusFound)
		return
	}
	id := r.PathValue("id")
	d, ok := s.inv.Device(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if d.Mode == config.ModeDirect {
		http.Error(w, "catalog-only device: connect your VNC client directly to "+d.Host+":"+fmt.Sprint(d.Port), http.StatusBadRequest)
		return
	}
	s.serveIndex(w)
}

// serveIndex returns the Vite entry point. Every other route is handled by the
// React Router inside it. If the tree was built without the frontend, say so
// instead of serving a blank 404.
func (s *Server) serveIndex(w http.ResponseWriter) {
	b, err := fs.ReadFile(s.dist, "index.html")
	if err != nil {
		http.Error(w, "frontend not built: run `npm --prefix frontend run build`", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	cfg := s.inv.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "abacad",
		"version": s.version,
		"listen":  cfg.Listen,
		"mdns":    cfg.MDNSEnabled(),
		"devices": len(s.inv.Devices()),
	})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": s.loggedIn(r)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.Token)), []byte(s.inv.Config().AdminToken)) != 1 {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "abacad_session",
		Value:    s.sessionValue(),
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 60 * 60,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "abacad_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": s.inv.Devices(),
		"hub": map[string]any{
			"listen": s.inv.Config().Listen,
		},
	})
}

func (s *Server) addDevice(w http.ResponseWriter, r *http.Request) {
	var req inventory.AddRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := s.inv.Add(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"device": v})
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	var req inventory.UpdateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := s.inv.Update(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": v})
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.inv.Remove(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) probeDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.inv.Device(id); !ok {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	st := s.inv.Probe(r.Context(), id)
	s.inv.CheckAuth(r.Context(), id)
	if v, ok := s.inv.Device(id); ok {
		writeJSON(w, http.StatusOK, map[string]any{"status": st, "device": v})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": st})
}

func (s *Server) discovered(w http.ResponseWriter, r *http.Request) {
	if s.disc == nil {
		writeJSON(w, http.StatusOK, map[string]any{"devices": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": s.disc.Entries()})
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if !s.loggedIn(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if _, ok := s.inv.Device(id); !ok {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		Subprotocols:       []string{"binary"},
	})
	if err != nil {
		return
	}
	c.SetReadLimit(1 << 20)
	wc := gateway.NewWSConn(r.Context(), c)
	if err := s.gw.HandleWS(r.Context(), wc, id); err != nil {
		s.log.Debug("browser session ended", "device", id, "err", err)
	}
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.loggedIn(r) {
			writeErr(w, http.StatusUnauthorized, "login required")
			return
		}
		next(w, r)
	}
}

func (s *Server) loggedIn(r *http.Request) bool {
	c, err := r.Cookie("abacad_session")
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.sessionValue())) == 1
}

func (s *Server) sessionValue() string {
	mac := hmac.New(sha256.New, []byte(s.inv.Config().CookieSecret))
	_, _ = mac.Write([]byte("abacad-session-v1:" + s.inv.Config().AdminToken))
	return hex.EncodeToString(mac.Sum(nil))
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

func immutable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}
