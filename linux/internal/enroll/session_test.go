package enroll

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestFreshDeviceMakesNoRequests is the load-bearing test for this package: on a
// machine nobody has set up, starting the client must not talk to the relay at
// all. Not "must not register" — must not send a single request, because even a
// bare connection tells a server this machine exists, and registering also hands
// over its hostname.
//
// The counter is the assertion. Anything that reintroduces an eager call — a
// capability probe, a version check, a "is this relay alive" ping — fails here,
// which is the point: the guarantee is about traffic, not about one function.
func TestFreshDeviceMakesNoRequests(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Errorf("relay was contacted before setup: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	s := &Session{Relay: srv.URL, Platform: "linux", Name: "box", Version: "0.5.2"}
	_, err := s.Run(context.Background(), "", "", false)

	if !errors.Is(err, ErrSetupRequired) {
		t.Fatalf("want ErrSetupRequired, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("made %d request(s) to the relay before a human asked for setup", n)
	}
}

// TestPersistNotCalledBeforeSetup: nothing should be written to disk either. A
// config file appearing on first launch is its own kind of surprise, and an
// empty credential written there would make Configured() lie.
func TestPersistNotCalledBeforeSetup(t *testing.T) {
	persisted := false
	s := &Session{
		Relay:   "https://relay.invalid",
		Persist: func(relay, deviceID, token string) error { persisted = true; return nil },
	}
	if _, err := s.Run(context.Background(), "", "", false); !errors.Is(err, ErrSetupRequired) {
		t.Fatalf("want ErrSetupRequired, got %v", err)
	}
	if persisted {
		t.Fatal("wrote credentials to disk before a human asked for setup")
	}
}

// TestSetupRegistersWhenAsked is the other half: once the human presses the
// button, the identical call registers, polls, and reports the claim code. If
// this passes while the two above pass, the gate is a gate and not a wall.
func TestSetupRegistersWhenAsked(t *testing.T) {
	// The claim goes through on the FIRST heartbeat deliberately. Interval()
	// floors the poll at 20s, so making this wait for a second one would buy a
	// 20-second test and no extra coverage — the register response already
	// carries the claim code, which is the thing being asserted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices/register":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"device_id":"abcdefghijklmnop","device_token":"abd_dev_x",
				"claim_code":"WXYZ-2K7M","claim_expires_in":300,"heartbeat_in":20}`))
		case "/api/devices/self/heartbeat":
			w.Write([]byte(`{"claimed":true,"device_id":"abcdefghijklmnop","claimed_by":"li@example.com"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	var gotID, gotCode, savedToken string
	s := &Session{
		Relay:    srv.URL,
		Platform: "linux",
		Name:     "box",
		Version:  "0.5.2",
		Persist:  func(relay, deviceID, token string) error { savedToken = token; return nil },
		OnCode:   func(deviceID, code string) { gotID, gotCode = deviceID, code },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := s.Run(ctx, "", "", true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if gotID != "abcdefghijklmnop" || gotCode != "WXYZ-2K7M" {
		t.Errorf("claim code not displayed: id=%q code=%q", gotID, gotCode)
	}
	if savedToken != "abd_dev_x" {
		t.Errorf("credential not persisted: %q", savedToken)
	}
	if res.ClaimedBy != "li@example.com" {
		t.Errorf("claimer not reported: %q", res.ClaimedBy)
	}
	if res.Token != "abd_dev_x" || res.DeviceID != "abcdefghijklmnop" {
		t.Errorf("unexpected result: %+v", res)
	}
}

// TestStoredCredentialResumesWithoutAsking: a device set up earlier must still
// reconnect on its own. The gate is about first contact, not about turning every
// launch into a prompt — allowRegister false with a token in hand goes straight
// to the heartbeat.
func TestStoredCredentialResumesWithoutAsking(t *testing.T) {
	registered := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devices/register" {
			registered = true
			t.Error("re-registered a device that already had a credential")
		}
		w.Write([]byte(`{"claimed":true,"device_id":"abcdefghijklmnop","claimed_by":"li@example.com"}`))
	}))
	defer srv.Close()

	s := &Session{Relay: srv.URL, Platform: "linux", Name: "box"}
	res, err := s.Run(context.Background(), "abd_dev_x", "abcdefghijklmnop", false)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if registered {
		t.Fatal("registered despite holding a credential")
	}
	if res.Token != "abd_dev_x" {
		t.Errorf("token changed across a resume: %q", res.Token)
	}
}

// TestHeartbeatGivesUp: an unreachable relay must eventually produce an error
// rather than retrying forever. This is what the GTK client's "Contacting …"
// state hangs on — without a cap the setup section shows a spinner with no
// button and no explanation for the life of the process.
func TestHeartbeatGivesUp(t *testing.T) {
	var beats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		beats.Add(1)
		w.WriteHeader(http.StatusInternalServerError) // transient, not UnknownToken
	}))
	defer srv.Close()

	s := &Session{Relay: srv.URL, retryDelay: time.Millisecond}
	_, err := s.Run(context.Background(), "abd_dev_x", "abcdefghijklmnop", false)

	if err == nil {
		t.Fatal("want an error from an unreachable relay, got nil")
	}
	if errors.Is(err, ErrSetupRequired) {
		t.Fatalf("wrong error: a live-but-failing relay is not a setup prompt: %v", err)
	}
	if n := beats.Load(); n != maxTransient {
		t.Errorf("gave up after %d heartbeats, want %d", n, maxTransient)
	}
}

// TestDeadCredentialDoesNotSilentlyReRegister: when the relay rejects a stored
// token on an unattended resume, the second pass must stop at the gate. Minting
// a fresh identity — a new device row, the hostname sent again — is exactly the
// thing that needs consent, and "the old one expired" does not supply it.
func TestDeadCredentialDoesNotSilentlyReRegister(t *testing.T) {
	var registered atomic.Bool
	var wiped atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devices/register" {
			registered.Store(true)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"device_id":"n","device_token":"t","claim_code":"C","heartbeat_in":1}`))
			return
		}
		w.WriteHeader(http.StatusNotFound) // ErrUnknownToken: credential is dead
	}))
	defer srv.Close()

	s := &Session{
		Relay: srv.URL,
		Persist: func(relay, deviceID, token string) error {
			if deviceID == "" && token == "" {
				wiped.Store(true)
			}
			return nil
		},
	}
	_, err := s.Run(context.Background(), "abd_dev_dead", "abcdefghijklmnop", false)

	if !errors.Is(err, ErrSetupRequired) {
		t.Fatalf("want ErrSetupRequired after a dead credential, got %v", err)
	}
	if registered.Load() {
		t.Fatal("silently re-registered after the relay rejected the stored token")
	}
	if !wiped.Load() {
		t.Error("dead credential was not cleared from the config")
	}
}
