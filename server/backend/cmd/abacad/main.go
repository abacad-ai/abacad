// Command abacad is the Abacad relay server: the MCP endpoint agents talk to,
// the WebSocket relay devices dial into, the dashboard API, and (Phase 6) the
// dashboard SPA — all on one port.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"abacad/internal/api"
	"abacad/internal/auth"
	"abacad/internal/config"
	"abacad/internal/device"
	"abacad/internal/mcp"
	"abacad/internal/relay"
	"abacad/internal/resolver"
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

	if cfg.Seed {
		seed(st)
	}

	hub := relay.NewHub()
	factory := &resolver.Factory{Store: st, Hub: hub}

	// /device: authenticate the device by its ?token=, register under its real
	// device id, and mark it seen.
	deviceHandler := &device.Handler{
		Hub: hub,
		Resolve: func(r *http.Request) (string, error) {
			token := r.URL.Query().Get("token")
			if token == "" {
				return "", errors.New("missing device token")
			}
			d, err := st.DeviceByTokenHash(auth.HashToken(token))
			if err != nil {
				return "", errors.New("unknown device token")
			}
			return d.ID, nil
		},
		OnSeen: st.TouchDevice,
	}

	// /mcp: authenticate the agent by its bearer MCP token -> account -> resolver.
	mcpHandler := &mcp.Handler{
		ResolverFor: func(r *http.Request) (mcp.DeviceResolver, error) {
			token := auth.BearerToken(r)
			if token == "" {
				return nil, errors.New("missing bearer token (add your Abacad MCP token as Authorization: Bearer …)")
			}
			acc, err := st.AccountByMCPTokenHash(auth.HashToken(token))
			if err != nil {
				return nil, errors.New("invalid MCP token")
			}
			return factory.For(acc.ID), nil
		},
	}

	apiHandler := (&api.API{Store: st, Hub: hub}).Handler()

	spa, err := web.New()
	if err != nil {
		log.Fatalf("web: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /mcp", mcpHandler)
	mux.HandleFunc("GET /mcp", methodNotAllowedMCP)
	mux.HandleFunc("DELETE /mcp", methodNotAllowedMCP)
	mux.Handle("GET /device", deviceHandler)
	mux.Handle("/api/", apiHandler)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"devices_online":%d}`, len(hub.OnlineIDs()))
	})
	// Methodless catch-all: the dashboard SPA (static assets + index.html
	// fallback for client routes). Dominated by the routes above under Go 1.22's
	// ServeMux precedence, so it only handles otherwise-unmatched paths.
	mux.Handle("/", spa)

	var handler http.Handler = logRequests(mux)
	if cfg.DevCORS {
		handler = devCORS(handler)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("agent MCP endpoint : POST %s/mcp   (Authorization: Bearer <mcp-token>)", cfg.Addr)
		log.Printf("device WebSocket   : %s/device?token=<device-token>", cfg.Addr)
		log.Printf("dashboard API      : %s/api/…", cfg.Addr)
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
	_, devToken, err := st.CreateDevice(acc.ID, "Seed device")
	if err != nil {
		log.Fatalf("seed device: %v", err)
	}
	mcpToken, err := st.RotateMCPToken(acc.ID)
	if err != nil {
		log.Fatalf("seed mcp token: %v", err)
	}
	log.Printf("SEED account=%s (%s / %s)", acc.ID, email, pass)
	log.Printf("SEED device_token=%s", devToken)
	log.Printf("SEED mcp_token=%s", mcpToken)
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
