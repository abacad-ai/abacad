// Command abacad is the headless Linux device client. It dials the abacad relay
// over a WebSocket and drives this machine on command — capture the screen and
// inject mouse/keyboard input over X11. It speaks the same wire contract as the
// macOS client (desktop verbs) plus the mobile verbs mapped onto desktop input.
//
// Config precedence (highest first): flags, environment, ~/.config/abacad/config.
//
//	--server-url / ABACAD_SERVER_URL   wss://host/device[?token=…]
//	--token      / ABACAD_TOKEN        device token (or embed ?token= in the URL)
package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"abacad-linux/internal/agent"
	"abacad-linux/internal/x11"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("abacad: ")

	var flagURL, flagToken string
	flag.StringVar(&flagURL, "server-url", "", "relay device URL, wss://host/device[?token=…]")
	flag.StringVar(&flagToken, "token", "", "device token (alternative to ?token= in the URL)")
	flag.Parse()

	cfg := loadConfigFile()
	serverURL := firstNonEmpty(flagURL, os.Getenv("ABACAD_SERVER_URL"), cfg["server_url"])
	token := firstNonEmpty(flagToken, os.Getenv("ABACAD_TOKEN"), cfg["token"])
	if serverURL == "" {
		log.Fatal("no server URL — set --server-url, ABACAD_SERVER_URL, or server_url in ~/.config/abacad/config")
	}
	if token != "" && !strings.Contains(serverURL, "token=") {
		serverURL = appendToken(serverURL, token)
	}

	x, err := x11.Open()
	if err != nil {
		log.Fatalf("cannot open display: %v", err)
	}
	defer x.Close()
	w, h := x.Size()
	log.Printf("X11 display ready (%dx%d)", w, h)

	a, err := agent.New(serverURL, x)
	if err != nil {
		log.Fatalf("agent: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("connecting to relay…")
	a.Run(ctx)
	log.Printf("shutting down")
}

// loadConfigFile reads ~/.config/abacad/config as KEY=VALUE lines (# comments,
// blanks ignored). A missing file is fine — it returns an empty map.
func loadConfigFile() map[string]string {
	out := map[string]string{}
	dir, err := os.UserConfigDir()
	if err != nil {
		return out
	}
	f, err := os.Open(filepath.Join(dir, "abacad", "config"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func appendToken(rawURL, token string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "token=" + token
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
