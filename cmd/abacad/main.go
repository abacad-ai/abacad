package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"abacad/internal/config"
	"abacad/internal/discovery"
	"abacad/internal/gateway"
	"abacad/internal/inventory"
	mcpserver "abacad/internal/mcp"
	"abacad/internal/web"
)

// version is overridden at build time with -X main.version=<tag>.
var version = "0.1.0"

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version":
			fmt.Println("abacad", version)
			return
		case "help", "-h", "--help":
			usage()
			return
		case "serve":
			args = args[1:]
		default:
			if !strings.HasPrefix(args[0], "-") {
				fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
				usage()
				os.Exit(2)
			}
		}
	}
	if err := cmdServe(args); err != nil {
		fmt.Fprintln(os.Stderr, "abacad:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `abacad — a small LAN VNC hub

Usage:
  abacad [serve] [--config PATH] [--listen ADDR] [--no-mdns]
                 [--tls-cert FILE --tls-key FILE] [-v]
  abacad version

Environment (overrides the config file, flags win over both):
  ABACAD_CONFIG      config file path
  ABACAD_LISTEN      listen address, e.g. 0.0.0.0:8787
  ABACAD_ADMIN_TOKEN admin token (at least 16 characters, never persisted)
  ABACAD_MDNS        true/false; false disables discovery and advertising

Everything else — adding, editing, and removing devices — is in the web UI.
Agents use MCP at /mcp on the same hub.
`)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", os.Getenv(config.EnvConfig), "config file (default $"+config.EnvConfig+")")
	listen := fs.String("listen", "", "listen address (overrides config)")
	noMDNS := fs.Bool("no-mdns", false, "disable mDNS discovery/advertising")
	tlsCert := fs.String("tls-cert", "", "TLS certificate (PEM) for HTTPS/WSS")
	tlsKey := fs.String("tls-key", "", "TLS private key (PEM)")
	verbose := fs.Bool("v", false, "debug logging")
	_ = fs.Parse(args)

	if (*tlsCert == "") != (*tlsKey == "") {
		return errors.New("--tls-cert and --tls-key must be set together")
	}
	scheme := "http"
	if *tlsCert != "" {
		scheme = "https"
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if err := cfg.ApplyEnv(os.Getenv); err != nil {
		return err
	}
	if *listen != "" {
		cfg.Listen = *listen
		if err := cfg.Save(); err != nil {
			return err
		}
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	inv := inventory.New(cfg)
	gw := gateway.New(inv, log)
	inv.OnChange(func() { gw.Sync(inv.Devices()) })

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	gw.Sync(inv.Devices())

	var disc *discovery.Discovery
	if cfg.MDNSEnabled() && !*noMDNS {
		disc = discovery.New(log)
		disc.Start(ctx, discovery.ListenPort(cfg.Listen), discovery.LocalHostname())
	} else {
		disc = discovery.New(log)
	}

	baseURL := scheme + "://" + displayAddr(cfg.Listen)
	mcpSrv := mcpserver.New(inv, version)
	handler := web.New(inv, gw, disc, mcpSrv, log, version).Handler()
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			// Interactive RFB traffic is small and latency-sensitive; Nagle
			// would coalesce wheel updates into visible stutter.
			switch conn := c.(type) {
			case *net.TCPConn:
				_ = conn.SetNoDelay(true)
			case *tls.Conn:
				if tc, ok := conn.NetConn().(*net.TCPConn); ok {
					_ = tc.SetNoDelay(true)
				}
			}
			return ctx
		},
	}

	go inv.Run(ctx)
	go func() {
		log.Info("abacad ready", "url", baseURL, "mcp", baseURL+"/mcp", "devices", len(inv.Devices()))
		fmt.Printf("\n  abacad is running\n  → %s\n  MCP: %s/mcp  (Authorization: Bearer <admin token>)\n  admin token: %s\n\n", baseURL, baseURL, cfg.AdminToken)
		var serveErr error
		if *tlsCert != "" {
			serveErr = srv.ServeTLS(ln, *tlsCert, *tlsKey)
		} else {
			serveErr = srv.Serve(ln)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error("http server stopped", "err", serveErr)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	_ = srv.Shutdown(shutdownCtx)
	gw.Close()
	if disc != nil {
		disc.Close()
	}
	return nil
}

func displayAddr(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = localIP()
	}
	return net.JoinHostPort(host, port)
}

func localIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	type candidate struct {
		name string
		ip   net.IP
	}
	var out []candidate
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || !ip.IsPrivate() {
				continue
			}
			out = append(out, candidate{name: iface.Name, ip: ip})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Prefer en0/eth0/wlan0 over virtual interfaces.
		score := func(n string) int {
			switch {
			case strings.HasPrefix(n, "en"), strings.HasPrefix(n, "eth"), strings.HasPrefix(n, "wlan"):
				return 0
			case strings.HasPrefix(n, "utun"), strings.HasPrefix(n, "tun"), strings.HasPrefix(n, "docker"), strings.HasPrefix(n, "br-"):
				return 2
			default:
				return 1
			}
		}
		return score(out[i].name) < score(out[j].name)
	})
	if len(out) > 0 {
		return out[0].ip.String()
	}
	return "127.0.0.1"
}
