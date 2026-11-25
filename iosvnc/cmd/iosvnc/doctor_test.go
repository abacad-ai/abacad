package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTunnels(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`[]`, "ready, no tunnels yet"},
		{`[{"udid":"a"},{"udid":"b"}]`, "ready, 2 tunnel(s)"},
		{`not json`, "ready"},
	}
	for _, c := range cases {
		if got := parseTunnels([]byte(c.body)); got != c.want {
			t.Errorf("parseTunnels(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

func TestCheckTunnelReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Errorf("path = %q, want /ready", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"udid":"x"}]`))
	}))
	defer srv.Close()

	c := checkTunnel(strings.TrimPrefix(srv.URL, "http://"))
	if !c.ok {
		t.Fatalf("tunnel check failed: %+v", c)
	}
	if !strings.Contains(c.detail, "1 tunnel") {
		t.Fatalf("detail = %q", c.detail)
	}
}

// TestCheckTunnelNotReady covers the window where the daemon is up but has not
// finished pairing: it is not a pass, and the hint must say to wait.
func TestCheckTunnelNotReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := checkTunnel(strings.TrimPrefix(srv.URL, "http://"))
	if c.ok {
		t.Fatal("a non-ready daemon was reported as ok")
	}
	if !strings.Contains(c.detail, "not ready") {
		t.Fatalf("detail = %q", c.detail)
	}
	if c.hint == "" {
		t.Fatal("no hint for a not-ready tunnel")
	}
}

func TestCheckTunnelMissing(t *testing.T) {
	// Port 1 is never a tunnel daemon.
	c := checkTunnel("127.0.0.1:1")
	if c.ok {
		t.Fatal("a missing daemon was reported as ok")
	}
	if !strings.Contains(c.hint, "ios") {
		t.Fatalf("hint does not name the command: %q", c.hint)
	}
}

func TestPrintChecksReportsEveryLine(t *testing.T) {
	// printChecks writes to stdout; this only guards against a panic on empty and
	// mixed input, which is what a partially failing run produces.
	printChecks(nil)
	printChecks([]check{{name: "a", detail: "b", ok: true, required: true}})
	printChecks([]check{{name: "a much longer name", detail: "b", ok: false, required: true, hint: "fix it"}})
}
