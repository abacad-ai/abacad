package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"abacad/internal/auth"
	"abacad/internal/store"
)

// waitForTrail polls the account trail until it has at least n rows. The recorder
// writes asynchronously, so a bare read races the write goroutine.
func waitForTrail(t *testing.T, st *store.Store, accountID string, n int) []store.Activity {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := st.Activities(accountID, store.ActivityFilter{})
		if err != nil {
			t.Fatalf("read trail: %v", err)
		}
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("trail has %d row(s), want %d", len(got), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A sign-in is the row a user is most likely to inspect when something looks
// wrong, so it must carry where it came from — and be attributed to the session it
// creates, whose cookie only exists on the response.
func TestLoginRecordsProvenance(t *testing.T) {
	a, _ := enrollFixture(t)
	a.logins = newLoginLimiter()
	pw, err := auth.HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := a.Store.CreateAccount("user@x.test", pw); err != nil {
		t.Fatalf("account: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"user@x.test","password":"hunter2"}`))
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	r.Header.Set("User-Agent", "Mozilla/5.0 (test)")
	w := httptest.NewRecorder()
	a.login(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("login: got %d, body %s", w.Code, w.Body.String())
	}

	acc, err := a.Store.AccountByEmail("user@x.test")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	rows := waitForTrail(t, a.Store, acc.ID, 1)
	row := rows[0]
	if row.Kind != "auth.login" {
		t.Fatalf("want auth.login, got %+v", row)
	}
	// Left-most forwarded hop, not the proxy's own address.
	if row.IP != "203.0.113.9" {
		t.Errorf("IP = %q, want the left-most X-Forwarded-For hop", row.IP)
	}
	if row.UserAgent != "Mozilla/5.0 (test)" {
		t.Errorf("UserAgent = %q", row.UserAgent)
	}
	if row.ActorKind != store.ActorSession {
		t.Errorf("ActorKind = %q, want %q", row.ActorKind, store.ActorSession)
	}
	if row.ActorLabel != "user@x.test" {
		t.Errorf("ActorLabel = %q, want the account email", row.ActorLabel)
	}
	// Attributed to the session it just minted, and as a fingerprint — never the
	// raw cookie value, which this endpoint hands to the browser in Set-Cookie.
	if row.ActorID == "" {
		t.Fatal("login should be attributed to the session it created")
	}
	var sid string
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			sid = c.Value
		}
	}
	if sid == "" {
		t.Fatal("no session cookie was set")
	}
	if strings.Contains(row.ActorID, sid) {
		t.Errorf("actor id leaks the live session cookie: %q", row.ActorID)
	}
	if row.ActorID != auth.SessionFingerprint(sid) {
		t.Errorf("ActorID = %q, want the fingerprint of the new session", row.ActorID)
	}
}

// A failed sign-in has no actor by definition: someone typed an email, and we do
// not know who. Naming the account owner would assert the opposite of what
// happened, so the row must carry provenance without an identity.
func TestFailedLoginHasNoActor(t *testing.T) {
	a, _ := enrollFixture(t)
	a.logins = newLoginLimiter()
	pw, err := auth.HashPassword("right")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := a.Store.CreateAccount("victim@x.test", pw); err != nil {
		t.Fatalf("account: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"victim@x.test","password":"wrong"}`))
	r.RemoteAddr = "198.51.100.7:4444"
	w := httptest.NewRecorder()
	a.login(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}

	acc, err := a.Store.AccountByEmail("victim@x.test")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	rows := waitForTrail(t, a.Store, acc.ID, 1)
	row := rows[0]
	if row.Kind != "auth.login_failed" {
		t.Fatalf("want auth.login_failed, got %+v", row)
	}
	if row.IP != "198.51.100.7" {
		t.Errorf("IP = %q, want the peer address", row.IP)
	}
	if row.ActorKind != "" || row.ActorID != "" || row.ActorLabel != "" {
		t.Errorf("failed sign-in must not claim an actor, got kind=%q id=%q label=%q",
			row.ActorKind, row.ActorID, row.ActorLabel)
	}
}

// Control-plane actions taken with a session cookie are attributed to it.
func TestControlPlaneActionRecordsSessionActor(t *testing.T) {
	a, acc := enrollFixture(t)
	sid, err := a.Store.CreateSession(acc.ID, "ua", time.Hour)
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	r := withAccount(httptest.NewRequest("POST", "/api/devices", strings.NewReader(`{"name":"box"}`)), acc)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: sid})
	r.Header.Set("X-Forwarded-For", "203.0.113.44")
	w := httptest.NewRecorder()
	a.createDevice(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create device: got %d, body %s", w.Code, w.Body.String())
	}

	rows := waitForTrail(t, a.Store, acc.ID, 1)
	row := rows[0]
	if row.ActorKind != store.ActorSession || row.ActorID != auth.SessionFingerprint(sid) {
		t.Errorf("want session actor %q, got kind=%q id=%q",
			auth.SessionFingerprint(sid), row.ActorKind, row.ActorID)
	}
	if row.IP != "203.0.113.44" {
		t.Errorf("IP = %q", row.IP)
	}
	if row.ActorLabel != acc.Email {
		t.Errorf("ActorLabel = %q, want %q", row.ActorLabel, acc.Email)
	}
}
