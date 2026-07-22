package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"abacad-linux/internal/x11"
)

// runConnect implements `abacad connect`: the device-authorization grant (RFC
// 8628) that enrolls this machine without copy-pasting a token. It asks the
// server for a short code, prints the URL to approve it, polls until the human
// approves in their browser, then writes the issued token to the config file so
// a plain `abacad` afterwards just runs.
func runConnect(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	server := fs.String("server", "https://abacad.ai", "abacad server base URL")
	_ = fs.Parse(args)

	base := strings.TrimRight(strings.TrimSpace(*server), "/")
	if base == "" {
		log.Fatal("empty --server")
	}

	platform := detectPlatform()
	log.Printf("this device: %s", platform)

	start, err := pairStart(base, platform)
	if err != nil {
		log.Fatalf("start pairing: %v", err)
	}

	link := start.VerificationURIComplete
	if link == "" {
		link = start.VerificationURI
	}
	fmt.Fprintf(os.Stderr, "\nTo connect this device, open:\n\n    %s\n\n", link)
	fmt.Fprintf(os.Stderr, "and approve it while signed in (code: %s). Waiting…\n", start.UserCode)

	tok, err := pairPoll(base, start)
	if err != nil {
		log.Fatalf("pairing: %v", err)
	}

	serverURL, token, err := splitToken(tok.WssURL)
	if err != nil {
		log.Fatalf("parse issued URL: %v", err)
	}
	path, err := saveConfig(map[string]string{"server_url": serverURL, "token": token})
	if err != nil {
		log.Fatalf("save config: %v", err)
	}

	fmt.Fprintf(os.Stderr, "\n✓ Approved. Saved credentials to %s\n", path)
	fmt.Fprintf(os.Stderr, "  Start the device now with:  abacad\n")
}

// detectPlatform reports "linux" when an X server is reachable and
// "linux-headless" otherwise, so a display-less box advertises shell-only and
// the agent won't try to screenshot a rack server.
func detectPlatform() string {
	if x, err := x11.Open(); err == nil {
		x.Close()
		return "linux"
	}
	return "linux-headless"
}

type startResp struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

type pollResp struct {
	Status      string `json:"status"`
	DeviceToken string `json:"device_token"`
	WssURL      string `json:"wss_url"`
	Error       string `json:"error"`
	Interval    int    `json:"interval"`
}

func pairStart(base, platform string) (*startResp, error) {
	body, _ := json.Marshal(map[string]string{"platform": platform})
	resp, err := http.Post(base+"/api/devices/pair/start", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server said %s: %s", resp.Status, serverError(data))
	}
	var s startResp
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.DeviceCode == "" {
		return nil, fmt.Errorf("server returned no device_code")
	}
	return &s, nil
}

// pairPoll waits for approval, honoring the server's interval hint and giving up
// after the pairing's stated lifetime.
func pairPoll(base string, start *startResp) (*pollResp, error) {
	interval := time.Duration(start.Interval) * time.Second
	if interval < time.Second {
		interval = 3 * time.Second
	}
	deadline := time.Now().Add(time.Duration(max(start.ExpiresIn, 60)) * time.Second)
	body, _ := json.Marshal(map[string]string{"device_code": start.DeviceCode})

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for approval")
		}
		resp, err := http.Post(base+"/api/devices/pair/poll", "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			var p pollResp
			if err := json.Unmarshal(data, &p); err != nil {
				return nil, err
			}
			if p.WssURL == "" {
				return nil, fmt.Errorf("server approved but returned no wss_url")
			}
			return &p, nil
		case http.StatusAccepted: // still pending
			time.Sleep(interval)
		default: // 403 denied / 404 unknown / 410 expired-or-used → terminal
			return nil, fmt.Errorf("%s", serverError(data))
		}
	}
}

// splitToken separates the issued wss URL into the token-free base URL and the
// token, matching the config's split server_url / token keys and keeping the
// secret out of the URL the daemon dials.
func splitToken(wssURL string) (base, token string, err error) {
	u, err := url.Parse(wssURL)
	if err != nil {
		return "", "", err
	}
	q := u.Query()
	token = q.Get("token")
	q.Del("token")
	u.RawQuery = q.Encode()
	return u.String(), token, nil
}

// saveConfig writes ~/.config/abacad/config with 0600 perms (the token is a
// bearer secret; there is no keychain on a headless box, so a private file is
// the right store — same trust model as an SSH key).
func saveConfig(kv map[string]string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(dir, "abacad")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(d, "config")
	var b strings.Builder
	b.WriteString("# Written by `abacad connect`. Re-run it to replace these.\n")
	for _, k := range []string{"server_url", "token"} {
		if v := kv[k]; v != "" {
			fmt.Fprintf(&b, "%s = %s\n", k, v)
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// serverError pulls the {"error": …} message out of a JSON error body, falling
// back to the raw text.
func serverError(data []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		return e.Error
	}
	return strings.TrimSpace(string(data))
}
