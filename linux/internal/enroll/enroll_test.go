package enroll

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices/register" || r.Method != "POST" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"device_id":"abcdefghijklmnop","device_token":"abd_dev_x",
			"claim_code":"WXYZ-2K7M","claim_expires_in":300,"heartbeat_in":20}`))
	}))
	defer srv.Close()

	reg, err := New(srv.URL).Register("linux", "box", "0.4.0")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if reg.DeviceID != "abcdefghijklmnop" || reg.DeviceToken != "abd_dev_x" || reg.ClaimCode != "WXYZ-2K7M" {
		t.Fatalf("unexpected registration: %+v", reg)
	}
}

// TestRegisterOldServer: a relay that predates self-enrollment must produce a
// clean fallback signal, not an opaque failure — otherwise a self-hoster who
// upgrades clients before the server gets a bricked client.
func TestRegisterOldServer(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := New(srv.URL).Register("linux", "box", "")
		srv.Close()
		if !errors.Is(err, ErrNotSupported) {
			t.Fatalf("status %d: want ErrNotSupported, got %v", status, err)
		}
	}
}

// TestRegisterIncomplete: a malformed 201 must not be mistaken for success, or
// the client would persist an empty token and never recover.
func TestRegisterIncomplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"claim_code":"WXYZ-2K7M"}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL).Register("linux", "box", ""); err == nil {
		t.Fatal("expected an error for a registration with no id/token")
	}
}

func TestHeartbeat(t *testing.T) {
	var gotAuth string
	claimed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		// The secret must never ride in the URL on this endpoint.
		if r.URL.RawQuery != "" {
			t.Errorf("heartbeat must not put anything in the query, got %q", r.URL.RawQuery)
		}
		if claimed {
			w.Write([]byte(`{"claimed":true,"device_id":"abcdefghijklmnop","name":"box","claimed_by":"a@x.test"}`))
			return
		}
		w.Write([]byte(`{"claimed":false,"device_id":"abcdefghijklmnop","claim_code":"WXYZ-2K7M","claim_expires_in":280,"heartbeat_in":20}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	st, err := c.Heartbeat("abd_dev_x")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if gotAuth != "Bearer abd_dev_x" {
		t.Fatalf("token must go in the Authorization header, got %q", gotAuth)
	}
	if st.Claimed || st.ClaimCode != "WXYZ-2K7M" {
		t.Fatalf("unclaimed status: %+v", st)
	}

	claimed = true
	st, err = c.Heartbeat("abd_dev_x")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !st.Claimed || st.ClaimedBy != "a@x.test" {
		t.Fatalf("claimed status must disclose the owner: %+v", st)
	}
}

// TestHeartbeatDeadCredential: every "we don't know you" shape must map to the
// one error that triggers wipe-and-re-register.
func TestHeartbeatDeadCredential(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone, http.StatusUnauthorized} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := New(srv.URL).Heartbeat("abd_dev_x")
		srv.Close()
		if !errors.Is(err, ErrUnknownToken) {
			t.Fatalf("status %d: want ErrUnknownToken, got %v", status, err)
		}
	}
}

func TestDeviceURL(t *testing.T) {
	cases := []struct{ relay, want string }{
		{"https://abacad.ai", "wss://abacad.ai/device"},
		{"https://abacad.ai/", "wss://abacad.ai/device"},
		{"http://127.0.0.1:8848", "ws://127.0.0.1:8848/device"},
		{"wss://relay.example.com", "wss://relay.example.com/device"},
		{"", "wss://abacad.ai/device"}, // empty falls back to the default relay
	}
	for _, c := range cases {
		got, err := DeviceURL(c.relay)
		if err != nil {
			t.Fatalf("DeviceURL(%q): %v", c.relay, err)
		}
		if got != c.want {
			t.Errorf("DeviceURL(%q) = %q, want %q", c.relay, got, c.want)
		}
	}
	if _, err := DeviceURL("ftp://nope"); err == nil {
		t.Error("expected an error for an unsupported scheme")
	}
}

// TestInterval: a missing or hostile hint must not spin the client.
func TestInterval(t *testing.T) {
	cases := []struct {
		in   int
		want time.Duration
	}{
		{0, 20 * time.Second},    // absent
		{-5, 20 * time.Second},   // nonsense
		{1, 20 * time.Second},    // too fast — would hammer the relay
		{30, 30 * time.Second},   // honored
		{99999, 5 * time.Minute}, // clamped
	}
	for _, c := range cases {
		if got := Interval(c.in); got != c.want {
			t.Errorf("Interval(%d) = %v, want %v", c.in, got, c.want)
		}
	}
}
