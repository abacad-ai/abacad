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
	"abacad-linux/internal/gui"
	"abacad-linux/internal/version"
	"abacad-linux/internal/x11"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("abacad: ")

	// `abacad connect` runs the device-authorization pairing flow and exits; a
	// bare `abacad` (below) runs the daemon from the saved config. Branch before
	// the daemon's flag set so connect owns its own flags.
	if len(os.Args) > 1 && os.Args[1] == "connect" {
		runConnect(os.Args[2:])
		return
	}

	var flagURL, flagToken string
	var flagGUI bool
	flag.StringVar(&flagURL, "server-url", "", "relay device URL, wss://host/device[?token=…]")
	flag.StringVar(&flagToken, "token", "", "device token (alternative to ?token= in the URL)")
	flag.BoolVar(&flagGUI, "gui", false, "run the GTK4/libadwaita desktop client instead of the headless daemon")
	flag.Parse()

	cfg := loadConfigFile()
	serverURL := firstNonEmpty(flagURL, os.Getenv("ABACAD_SERVER_URL"), cfg["server_url"])
	token := firstNonEmpty(flagToken, os.Getenv("ABACAD_TOKEN"), cfg["token"])
	// Assemble the dial URL; the agent lifts ?token= into a header and leaves other
	// query params (so ?version= rides through). For the GUI the URL may be empty —
	// the user enters it in-app — so guard the assembly and defer the "no URL" fatal
	// to the headless path below.
	if serverURL != "" && token != "" && !strings.Contains(serverURL, "token=") {
		serverURL = appendToken(serverURL, token)
	}
	if serverURL != "" && !strings.Contains(serverURL, "version=") {
		serverURL = appendParam(serverURL, "version", version.Version)
	}

	// A display is optional. On a box with no X server (a rack server, a
	// container, a cloud VM) we run shell-only: the daemon still holds the relay
	// socket and serves the SSH tunnel, and the dispatcher cleanly rejects the
	// screen/input verbs. Only Fatal-ing here would make headless boxes — the
	// whole point of a CLI client — impossible to run.
	x, err := x11.Open()
	if err != nil {
		log.Printf("no display (%v) — running shell-only; screen/input verbs are rejected, the SSH tunnel still works", err)
		x = nil
	} else {
		defer x.Close()
		w, h := x.Size()
		log.Printf("X11 display ready (%dx%d)", w, h)
	}

	// The GUI drives connections itself (Connect / Disconnect / Pause) and runs its
	// own GTK main loop, returning when the window closes. Only in `-tags gui`
	// builds is this the real libadwaita window; otherwise gui.Run reports that the
	// binary has no GUI.
	if flagGUI {
		if err := gui.Run(serverURL, x); err != nil {
			log.Fatalf("gui: %v", err)
		}
		return
	}

	if serverURL == "" {
		log.Fatal("no server URL — set --server-url, ABACAD_SERVER_URL, or server_url in ~/.config/abacad/config (or run with --gui)")
	}

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
	return appendParam(rawURL, "token", token)
}

// appendParam adds key=value to a URL's query, choosing ? or & as needed. Values
// here are our own version/token strings (no reserved chars), so no escaping.
func appendParam(rawURL, key, value string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + key + "=" + value
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
