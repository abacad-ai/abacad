// Command abacad is the abacad relay server: the MCP endpoint agents talk to,
// the WebSocket relay devices dial into, the dashboard API, and (Phase 6) the
// dashboard SPA — all on one port.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"abacad/internal/activity"
	"abacad/internal/api"
	"abacad/internal/auth"
	"abacad/internal/blob"
	"abacad/internal/config"
	"abacad/internal/connect"
	"abacad/internal/device"
	"abacad/internal/events"
	"abacad/internal/mcp"
	"abacad/internal/relay"
	"abacad/internal/resolver"
	"abacad/internal/screenshot"
	"abacad/internal/sshjump"
	"abacad/internal/store"
	"abacad/internal/web"
)

func main() {
	cfg := config.Load()
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[abacad] ")

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db %q: %v", cfg.DBPath, err)
	}
	defer st.Close()

	if err := os.MkdirAll(cfg.BlobDir, 0o755); err != nil {
		log.Fatalf("blob dir %q: %v", cfg.BlobDir, err)
	}

	shots, err := screenshot.Open(cfg.ScreenshotDir)
	if err != nil {
		log.Fatalf("screenshot dir %q: %v", cfg.ScreenshotDir, err)
	}

	if cfg.Seed {
		seed(st)
	}

	hub := relay.NewHub()
	evlog := events.NewLog()
	trail := activity.New(st, time.Duration(cfg.ActivityRetentionDays)*24*time.Hour)
	factory := &resolver.Factory{Store: st, Hub: hub}

	// /device: authenticate the device by its token, register under its real
	// device id, and mark it seen. The token is read from the Authorization
	// header first (preferred — keeps it out of URLs, proxy logs, and history)
	// and falls back to ?token= for older clients.
	deviceHandler := &device.Handler{
		Hub: hub,
		Resolve: func(r *http.Request) (string, string, error) {
			token := auth.BearerToken(r)
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token == "" {
				return "", "", errors.New("missing device token")
			}
			d, err := st.DeviceByTokenHash(auth.HashToken(token))
			if err != nil {
				return "", "", errors.New("unknown device token")
			}
			return d.ID, d.AccountID, nil
		},
		OnSeen:    st.TouchDevice,
		OnVersion: st.SetDeviceVersion,
		Events:    evlog,
		Activity:  trail,
	}

	// Browser devices dial the same /device WebSocket but from their own subdomain
	// (<device-id>.abacad.ai) with no token — the id in the Host is the addressing
	// and auth key. Same hub/trail as the token handler; only Resolve differs.
	deviceHostWS := &device.Handler{
		Hub: hub,
		Resolve: func(r *http.Request) (string, string, error) {
			id := deviceHostID(r.Host)
			if id == "" {
				return "", "", errors.New("not a device host")
			}
			d, err := st.DeviceByID(id)
			if err != nil {
				return "", "", errors.New("unknown device")
			}
			return d.ID, d.AccountID, nil
		},
		OnSeen:    st.TouchDevice,
		OnVersion: st.SetDeviceVersion,
		Events:    evlog,
		Activity:  trail,
	}

	// blobSvc is the account-scoped data-plane store, shared by the /blobs HTTP
	// handler and the MCP file-transfer tools (push_file / pull_file), which stage
	// and read blob bytes on the agent's behalf so the agent never leaves the MCP
	// surface to move a file.
	blobSvc := &blob.Service{Store: st, Dir: cfg.BlobDir, MaxBytes: cfg.MaxBlobBytes}

	// /mcp: authenticate the agent by its bearer API key -> account + scope ->
	// scoped resolver. The scope also gates which methods the key may call.
	mcpHandler := &mcp.Handler{
		Blobs: mcpBlobs{svc: blobSvc},
		ResolverFor: func(r *http.Request) (mcp.DeviceResolver, mcp.Scope, error) {
			token := auth.BearerToken(r)
			if token == "" {
				return nil, nil, errors.New("missing bearer token (add your abacad API key as Authorization: Bearer …)")
			}
			accID, scope, err := st.APIKeyScopeByTokenHash(auth.HashToken(token))
			if err != nil {
				return nil, nil, errors.New("invalid API key")
			}
			return factory.ForScope(accID, scope), scope, nil
		},
	}

	// /connect: a raw TCP tunnel to a device-reachable target. Same API-key
	// identity as /mcp, but accepts the token as ?token= too (matching the device
	// endpoint), since the agent-side client is a bare WebSocket that may not set
	// an Authorization header. The key's scope must permit tunnels.
	connectHandler := &connect.Handler{
		ResolverFor: func(r *http.Request) (mcp.DeviceResolver, string, store.KeyScope, error) {
			token := auth.BearerToken(r)
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token == "" {
				return nil, "", store.KeyScope{}, errors.New("missing API key")
			}
			accID, scope, err := st.APIKeyScopeByTokenHash(auth.HashToken(token))
			if err != nil {
				return nil, "", store.KeyScope{}, errors.New("invalid API key")
			}
			return factory.ForScope(accID, scope), accID, scope, nil
		},
		Activity: trail,
	}

	apiHandler := (&api.API{
		Store: st, Hub: hub, Events: evlog, Activity: trail, Shots: shots, BaseDomain: cfg.BaseDomain,
		DownloadsDir:   cfg.DownloadsDir,
		GoogleClientID: cfg.GoogleClientID, GoogleClientSecret: cfg.GoogleClientSecret, GoogleRedirectURL: cfg.GoogleRedirectURL,
	}).Handler()

	// /blobs: the data plane. Authorized by any of the server's identities —
	// dashboard session, API-key bearer, or device token — all resolving to the
	// owning account, since a device uploads (screenshots, files), an agent and
	// the dashboard download, and blobs are scoped per account.
	accountForBlob := func(r *http.Request) (store.Account, error) {
		if acc, err := st.AccountBySession(auth.SessionID(r)); err == nil {
			return acc, nil // dashboard session cookie
		}
		if tok := auth.BearerToken(r); tok != "" {
			if accID, _, err := st.APIKeyScopeByTokenHash(auth.HashToken(tok)); err == nil {
				return st.AccountByID(accID) // agent API-key bearer
			}
		}
		tok := r.URL.Query().Get("token") // device token (query, matching /device)
		if tok == "" {
			tok = auth.BearerToken(r)
		}
		if tok != "" {
			if d, err := st.DeviceByTokenHash(auth.HashToken(tok)); err == nil {
				return st.AccountByID(d.AccountID)
			}
		}
		return store.Account{}, errors.New("missing or invalid credentials (session, MCP token, or device token)")
	}
	blobHandler := &blob.Handler{Svc: blobSvc, Account: accountForBlob}

	spa, err := web.New()
	if err != nil {
		log.Fatalf("web: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /mcp", mcpHandler)
	mux.HandleFunc("GET /mcp", methodNotAllowedMCP)
	mux.HandleFunc("DELETE /mcp", methodNotAllowedMCP)
	mux.Handle("GET /device", deviceHandler)
	mux.Handle("GET /connect", connectHandler)
	mux.Handle("POST /blobs", http.HandlerFunc(blobHandler.Upload))
	mux.Handle("GET /blobs/{id}", http.HandlerFunc(blobHandler.Download))
	mux.Handle("/api/", apiHandler)
	mux.Handle("GET /downloads/{file}", downloadsHandler(cfg.DownloadsDir))
	// Vendored html2canvas for the browser client (referenced same-origin as
	// /_hc.js). On a device host the hostMux passes this path through to here.
	mux.Handle("GET /_hc.js", web.Html2Canvas())
	// Public legal pages (required by Google's OAuth consent screen).
	mux.Handle("GET /privacy", web.PrivacyPolicy())
	mux.Handle("GET /terms", web.TermsOfService())
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"devices_online":%d}`, len(hub.OnlineIDs()))
	})
	// Methodless catch-all: the dashboard SPA (static assets + index.html
	// fallback for client routes). Dominated by the routes above under Go 1.22's
	// ServeMux precedence, so it only handles otherwise-unmatched paths.
	mux.Handle("/", spa)

	// Browser devices are addressed by subdomain: <device-id>.abacad.ai. A 16-letter
	// device id can't collide with a system host (apex / app / api), so route by shape
	// — a leftmost [a-z]{16} label means "browser device": serve the client page at /
	// and authenticate its /device WebSocket by the id in the Host. Everything else
	// (native clients with ?token=, the dashboard, the API) falls through to the mux.
	browserPage := web.BrowserClient()
	hostMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deviceHostID(r.Host) != "" {
			switch r.URL.Path {
			case "/":
				browserPage.ServeHTTP(w, r)
				return
			case "/device":
				deviceHostWS.ServeHTTP(w, r)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})

	var handler http.Handler = logRequests(hostMux)
	if cfg.DevCORS {
		handler = devCORS(handler)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	// SSH jump host: makes ssh <device>.<base-domain> route to a device's own
	// sshd for a stock ProxyJump client. Opt-in via -ssh-addr / ABACAD_SSH_ADDR,
	// which may list several addresses (e.g. ":22,:443") so networks that only
	// allow egress on 443 can still reach it.
	if cfg.SSHAddr != "" {
		signer, err := sshjump.LoadOrCreateHostKey(cfg.SSHHostKey)
		if err != nil {
			log.Fatalf("ssh host key %q: %v", cfg.SSHHostKey, err)
		}
		jump := &sshjump.Server{
			BaseDomain: cfg.BaseDomain,
			HostSigner: signer,
			// Authorize the connection by public key -> account.
			AccountForKey: func(key ssh.PublicKey) (string, error) {
				acc, err := st.AccountBySSHKeyFingerprint(ssh.FingerprintSHA256(key))
				if err != nil {
					return "", err
				}
				return acc.ID, nil
			},
			// Route only to an owned, online device; pin the target to its sshd.
			OpenTunnel: func(ctx context.Context, accountID, deviceID string) (io.ReadWriteCloser, error) {
				dc, err := factory.For(accountID).Resolve(ctx, deviceID)
				if err != nil {
					return nil, err
				}
				s, err := dc.OpenStream(ctx, "127.0.0.1:22")
				if err == nil {
					trail.Record(store.Activity{
						AccountID: accountID, DeviceID: dc.DeviceID,
						Kind: activity.KindSSHSession, Source: "ssh",
					})
				}
				return s, err
			},
		}
		for _, addr := range strings.Split(cfg.SSHAddr, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				log.Fatalf("ssh jump listen %q: %v", addr, err)
			}
			log.Printf("ssh jump host      : ssh <device>.%s  (listening on %s)", cfg.BaseDomain, addr)
			go func(ln net.Listener) {
				if err := jump.Serve(ln); err != nil {
					log.Fatalf("ssh jump host: %v", err)
				}
			}(ln)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("agent MCP endpoint : POST %s/mcp   (Authorization: Bearer <api-key>)", cfg.Addr)
		log.Printf("device WebSocket   : %s/device?token=<device-token>", cfg.Addr)
		log.Printf("tunnel WebSocket   : %s/connect?token=<api-key>&device=<id>&target=host:port", cfg.Addr)
		log.Printf("blob store         : POST %s/blobs · GET %s/blobs/{id}   (session | API key | device token)", cfg.Addr, cfg.Addr)
		log.Printf("dashboard API      : %s/api/…", cfg.Addr)
		if cfg.GoogleEnabled() {
			log.Printf("google sign-in     : enabled  (callback %s)", func() string {
				if cfg.GoogleRedirectURL != "" {
					return cfg.GoogleRedirectURL
				}
				return "<origin>/api/auth/google/callback"
			}())
		}
		log.Printf("downloads          : GET %s/downloads/<file>   (from %s)", cfg.Addr, cfg.DownloadsDir)
		log.Printf("health             : GET %s/health", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down ...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// seed provisions a dev account + device + MCP token and prints the secrets, so
// a developer can drive the full loop without the dashboard.
func seed(st *store.Store) {
	const email, pass = "dev@abacad.local", "devpass"
	acc, err := st.AccountByEmail(email)
	if errors.Is(err, store.ErrNotFound) {
		hash, _ := auth.HashPassword(pass)
		acc, err = st.CreateAccount(email, hash)
	}
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	_, devToken, err := st.CreateDevice(acc.ID, "Seed device", "")
	if err != nil {
		log.Fatalf("seed device: %v", err)
	}
	apiKey, _, err := st.CreateAPIKey(acc.ID, "Seed key",
		store.KeyScope{AllDevices: true, AllMethods: true, AllowTunnel: true})
	if err != nil {
		log.Fatalf("seed api key: %v", err)
	}
	log.Printf("SEED account=%s (%s / %s)", acc.ID, email, pass)
	log.Printf("SEED device_token=%s", devToken)
	log.Printf("SEED api_key=%s", apiKey)
}

// deviceLabel matches a browser-device subdomain label: exactly the 16 lowercase
// letters that NewDeviceID() mints. System hosts (apex, app, api, www) never take
// this shape, so the leftmost label alone disambiguates a device host.
var deviceLabel = regexp.MustCompile(`^[a-z]{16}$`)

// deviceHostID returns the browser-device id in the request Host (its leftmost
// label, when that label is a device id), or "" when the Host is not a device
// subdomain. Any port is stripped first.
func deviceHostID(host string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	label := host
	if i := strings.IndexByte(host, '.'); i >= 0 {
		label = host[:i]
	}
	if deviceLabel.MatchString(label) {
		return label
	}
	return ""
}

// downloadsHandler serves public release artifacts (e.g. the macOS client dmg)
// from a plain directory, so publishing a build is just a file copy into the
// data volume — no restart. Flat namespace, files only, no listing.
func downloadsHandler(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.PathValue("file")) // Base strips any traversal
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, p)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path) // path only — never the query (carries tokens)
		next.ServeHTTP(w, r)
	})
}

// devCORS allows the Vite dev server and smoke scripts to hit Go directly. Only
// enabled with -dev-cors; never in production.
func devCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func methodNotAllowedMCP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = w.Write([]byte(`{"error":"method not allowed (stateless MCP: POST only)"}`))
}

// mcpBlobs adapts *blob.Service to mcp.BlobStore: the file-transfer tools want
// plain (id, size, sha256) tuples and a reader, not the store.Blob record, so
// the coupling between the two packages stays at this thin boundary.
type mcpBlobs struct{ svc *blob.Service }

func (b mcpBlobs) Put(accountID, contentType string, r io.Reader) (id string, size int64, sha256 string, err error) {
	bl, err := b.svc.Put(accountID, contentType, r)
	if err != nil {
		return "", 0, "", err
	}
	return bl.ID, bl.Size, bl.SHA256, nil
}

func (b mcpBlobs) Open(accountID, id string) (rc io.ReadCloser, size int64, sha256 string, err error) {
	f, bl, err := b.svc.Open(accountID, id)
	if err != nil {
		return nil, 0, "", err
	}
	return f, bl.Size, bl.SHA256, nil
}
