package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"text/tabwriter"
	"time"

	"abacad/internal/config"
	"abacad/internal/discovery"
	"abacad/internal/gateway"
	"abacad/internal/inventory"
	mcpserver "abacad/internal/mcp"
	"abacad/internal/rfb"
	"abacad/internal/web"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(os.Args[2:])
	case "add":
		err = cmdAdd(os.Args[2:])
	case "list", "ls":
		err = cmdList(os.Args[2:])
	case "remove", "rm":
		err = cmdRemove(os.Args[2:])
	case "probe":
		err = cmdProbe(os.Args[2:])
	case "mcp":
		err = cmdMCP(os.Args[2:])
	case "doctor":
		err = cmdDoctor(os.Args[2:])
	case "token":
		err = cmdToken(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("abacad", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "abacad:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `abacad — a small LAN VNC hub

Usage:
  abacad serve   [--config PATH] [--listen ADDR] [--no-mdns] [-v]
  abacad add     --name NAME --host HOST [--port 5900] [--password PW]
                 [--platform macos|linux|windows|android] [--mode gateway|l4|direct] [--expose]
  abacad list    [--config PATH] [--probe] [--json]
  abacad remove  ID
  abacad probe   [ID]
  abacad mcp     [--config PATH] [--host HOST] [--base-url URL]
  abacad doctor  [--config PATH]
  abacad token   [--config PATH] [--rotate]
  abacad version

Config defaults to ~/.config/abacad/config.yaml.
`)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	listen := fs.String("listen", "", "listen address (overrides config)")
	noMDNS := fs.Bool("no-mdns", false, "disable mDNS discovery/advertising")
	tlsCert := fs.String("tls-cert", "", "TLS certificate (PEM) for HTTPS/WSS")
	tlsKey := fs.String("tls-key", "", "TLS private key (PEM)")
	verbose := fs.Bool("v", false, "debug logging")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *listen != "" {
		cfg.Listen = *listen
		if err := cfg.Save(); err != nil {
			return err
		}
	}
	if (*tlsCert == "") != (*tlsKey == "") {
		return errors.New("--tls-cert and --tls-key must be set together")
	}
	scheme := "http"
	if *tlsCert != "" {
		scheme = "https"
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

	handler := web.New(inv, gw, disc, log, version).Handler()
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go inv.Run(ctx, 30*time.Second)
	go func() {
		url := scheme + "://" + displayAddr(cfg.Listen)
		log.Info("abacad ready", "url", url, "devices", len(inv.Devices()))
		fmt.Printf("\n  abacad is running\n  → %s\n  admin token: %s\n\n", url, cfg.AdminToken)
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

func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	name := fs.String("name", "", "device name")
	host := fs.String("host", "", "device host or IP")
	port := fs.Int("port", 5900, "device VNC port")
	password := fs.String("password", "", "device VNC password")
	platform := fs.String("platform", "", "macos|linux|windows|android")
	mode := fs.String("mode", config.ModeGateway, "gateway|l4|direct")
	expose := fs.Bool("expose", false, "open a raw VNC port for agents")
	_ = fs.Parse(args)
	if *host == "" {
		return errors.New("--host is required")
	}
	if *name == "" {
		*name = *host
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	inv := inventory.New(cfg)
	v, err := inv.Add(context.Background(), inventory.AddRequest{
		Name: *name, Host: *host, Port: *port, Password: *password,
		Platform: *platform, Mode: *mode, Expose: *expose,
	})
	if err != nil {
		return err
	}
	printDevice(v)
	if v.VNCPort > 0 {
		fmt.Printf("\n  agent endpoint: vnc://%s:%d  (password %s)\n", localIP(), v.VNCPort, v.Token)
	}
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	probe := fs.Bool("probe", false, "probe every device first")
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	inv := inventory.New(cfg)
	if *probe {
		inv.ProbeAll(context.Background())
	}
	devices := inv.Devices()
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(devices)
	}
	if len(devices) == 0 {
		fmt.Println("no devices yet — add one with `abacad add`")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tADDRESS\tMODE\tVNC\tONLINE\tPLATFORM")
	for _, d := range devices {
		vnc := "-"
		if d.VNCPort > 0 {
			vnc = fmt.Sprint(d.VNCPort)
		}
		online := "offline"
		if d.Online {
			online = "online"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s:%d\t%s\t%s\t%s\t%s\n", d.ID, d.Name, d.Host, d.Port, d.Mode, vnc, online, d.Platform)
	}
	_ = tw.Flush()
	return nil
}

func cmdRemove(args []string) error {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return errors.New("usage: abacad remove ID")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if err := cfg.RemoveDevice(fs.Arg(0)); err != nil {
		return err
	}
	fmt.Printf("removed %s\n", fs.Arg(0))
	return nil
}

func cmdProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	inv := inventory.New(cfg)
	if fs.NArg() == 0 {
		inv.ProbeAll(context.Background())
		for _, d := range inv.Devices() {
			printStatus(d)
		}
		return nil
	}
	for _, id := range fs.Args() {
		if _, ok := inv.Device(id); !ok {
			return fmt.Errorf("device %q not found", id)
		}
		inv.Probe(context.Background(), id)
		v, _ := inv.Device(id)
		printStatus(v)
	}
	return nil
}

func cmdMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	host := fs.String("host", "", "host agents should use to reach this hub (default: LAN IP)")
	baseURL := fs.String("base-url", "", "browser base URL (default http://HOST:PORT)")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	inv := inventory.New(cfg)
	inv.ProbeAll(context.Background())
	h := *host
	if h == "" {
		h = localIP()
	}
	base := *baseURL
	if base == "" {
		base = fmt.Sprintf("http://%s:%d", h, discovery.ListenPort(cfg.Listen))
	}
	return mcpserver.New(inv, h, strings.TrimRight(base, "/"), version).Run(context.Background())
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	fmt.Printf("config: %s\n", cfg.Path())
	if fi, err := os.Stat(cfg.Path()); err == nil {
		fmt.Printf("permissions: %s\n", fi.Mode().Perm())
	}
	fmt.Printf("listen: %s\n", cfg.Listen)
	if ln, err := net.Listen("tcp", cfg.Listen); err != nil {
		fmt.Printf("listen check: %v (is the hub already running?)\n", err)
	} else {
		_ = ln.Close()
		fmt.Println("listen check: ok")
	}
	fmt.Printf("admin token: %s\n", cfg.AdminToken)
	fmt.Printf("mdns: %v\n", cfg.MDNSEnabled())

	inv := inventory.New(cfg)
	devices := inv.Devices()
	if len(devices) == 0 {
		fmt.Println("devices: none")
		return nil
	}
	fmt.Printf("devices: %d\n", len(devices))
	inv.ProbeAll(context.Background())
	for _, d := range inv.Devices() {
		line := fmt.Sprintf("  %-16s %-20s %s", d.ID, d.Host+":"+fmt.Sprint(d.Port), d.Banner)
		if !d.Online {
			line += "  " + d.LastError
		}
		fmt.Println(strings.TrimRight(line, " "))
		if !d.Online {
			continue
		}
		addr := net.JoinHostPort(d.Host, fmt.Sprint(d.Port))
		_, types, err := rfb.SecurityTypes(context.Background(), addr, 3*time.Second)
		if err != nil {
			fmt.Printf("  %-16s security: %v\n", "", err)
			continue
		}
		fmt.Printf("  %-16s security: %s\n", "", rfb.DescribeTypes(types))
		if !rfb.SupportsStandardAuth(types) {
			fmt.Printf("  %-16s WARNING: no standard VNC auth offered; enable 'VNC viewers may control screen with password' or use TigerVNC\n", "")
		}
	}
	return nil
}

func cmdToken(args []string) error {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	rotate := fs.Bool("rotate", false, "generate a new admin token")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *rotate {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		cfg.AdminToken = hex.EncodeToString(b)
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Println("admin token rotated")
	}
	fmt.Println(cfg.AdminToken)
	return nil
}

func printDevice(v inventory.View) {
	fmt.Printf("%s  %s  %s:%d  mode=%s", v.ID, v.Name, v.Host, v.Port, v.Mode)
	if v.VNCPort > 0 {
		fmt.Printf("  vnc=%d token=%s", v.VNCPort, v.Token)
	}
	fmt.Println()
}

func printStatus(v inventory.View) {
	state := "offline"
	if v.Online {
		state = "online"
	}
	fmt.Printf("%-16s %-10s %s", v.ID, state, v.Banner)
	if !v.Online && v.LastError != "" {
		fmt.Printf("  (%s)", v.LastError)
	}
	fmt.Println()
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
