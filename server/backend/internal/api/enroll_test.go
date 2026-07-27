package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"abacad/internal/activity"
	"abacad/internal/auth"
	"abacad/internal/store"
)

// enrollFixture builds an API with the self-enrollment throttles initialized (a
// bare &API{} would nil-panic on them, since Handler() is what normally wires
// them up).
func enrollFixture(t *testing.T) (*API, store.Account) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	acc, err := st.CreateAccount("owner@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	a := &API{
		Store:         st,
		Activity:      activity.New(st, 0),
		registrations: newQuotaLimiter(3, time.Hour),
		claimAttempts: newQuotaLimiter(30, 15*time.Minute),
	}
	return a, acc
}

func postJSON(t *testing.T, h http.HandlerFunc, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.RemoteAddr = "203.0.113.7:1234"
	w := httptest.NewRecorder()
	h(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

// register self-registers one device and returns its id, token and claim code.
func register(t *testing.T, a *API) (id, token, code string) {
	t.Helper()
	w, out := postJSON(t, a.registerDevice, "/api/devices/register",
		`{"platform":"linux","name":"box","version":"0.4.0"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: got %d, body %s", w.Code, w.Body.String())
	}
	id, _ = out["device_id"].(string)
	token, _ = out["device_token"].(string)
	code, _ = out["claim_code"].(string)
	if id == "" || token == "" || code == "" {
		t.Fatalf("register response missing fields: %v", out)
	}
	return id, token, code
}

func withAccount(r *http.Request, acc store.Account) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), accountKey, acc))
}

// TestClaimRequiresConsent: claiming must clear the SAME consent bar as
// pairApprove — accepted:true, recorded under the same "enrollment.accepted"
// method so both enrollment routes read identically in the audit trail.
func TestClaimRequiresConsent(t *testing.T) {
	a, acc := enrollFixture(t)
	id, _, code := register(t, a)

	body := `{"device_id":"` + id + `","claim_code":"` + code + `","name":"box","accepted":false}`
	r := withAccount(httptest.NewRequest("POST", "/api/claim", strings.NewReader(body)), acc)
	w := httptest.NewRecorder()
	a.claimDevice(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accepted:false should be 422, got %d (%s)", w.Code, w.Body.String())
	}
	// A refused claim must not have created a device.
	if _, err := a.Store.DeviceOwnedBy(id, acc.ID); err != store.ErrNotFound {
		t.Fatalf("refused claim must not create a device, got %v", err)
	}

	body = `{"device_id":"` + id + `","claim_code":"` + code + `","name":"box","accepted":true}`
	r = withAccount(httptest.NewRequest("POST", "/api/claim", strings.NewReader(body)), acc)
	w = httptest.NewRecorder()
	a.claimDevice(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("claim: got %d (%s)", w.Code, w.Body.String())
	}
	if _, err := a.Store.DeviceOwnedBy(id, acc.ID); err != nil {
		t.Fatalf("claimed device should be owned: %v", err)
	}
	if !hasConsent(t, a.Store, acc.ID, "enrollment.accepted") {
		t.Fatal("claim must record the enrollment.accepted consent row")
	}
}

// TestPreviewIsNotAnOracle: every failure mode must be byte-identical, so the
// endpoint can't be used to discover which device ids exist.
func TestPreviewIsNotAnOracle(t *testing.T) {
	a, _ := enrollFixture(t)
	id, _, code := register(t, a)

	realIDWrongCode, b1 := postJSON(t, a.claimPreview, "/api/claim/preview",
		`{"device_id":"`+id+`","claim_code":"AAAA-BBBB"}`)
	unknownID, b2 := postJSON(t, a.claimPreview, "/api/claim/preview",
		`{"device_id":"zzzzzzzzzzzzzzzz","claim_code":"`+code+`"}`)

	if realIDWrongCode.Code != http.StatusNotFound || unknownID.Code != http.StatusNotFound {
		t.Fatalf("both should be 404, got %d and %d", realIDWrongCode.Code, unknownID.Code)
	}
	if b1["error"] != b2["error"] {
		t.Fatalf("responses distinguishable: %q vs %q", b1["error"], b2["error"])
	}

	// The correct pair works and reports the device without consuming the code.
	w, out := postJSON(t, a.claimPreview, "/api/claim/preview",
		`{"device_id":"`+id+`","claim_code":"`+code+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("valid preview: got %d (%s)", w.Code, w.Body.String())
	}
	if out["platform"] != "linux" {
		t.Fatalf("preview platform: %v", out["platform"])
	}
	if again, _ := postJSON(t, a.claimPreview, "/api/claim/preview",
		`{"device_id":"`+id+`","claim_code":"`+code+`"}`); again.Code != http.StatusOK {
		t.Fatalf("preview must not consume the code, second call got %d", again.Code)
	}
}

// TestPreviewForcesRotationAfterGuessing: repeated wrong codes against a real id
// must burn the code the attacker is guessing at.
func TestPreviewForcesRotationAfterGuessing(t *testing.T) {
	a, _ := enrollFixture(t)
	id, _, code := register(t, a)

	for i := 0; i < maxClaimAttempts; i++ {
		postJSON(t, a.claimPreview, "/api/claim/preview",
			`{"device_id":"`+id+`","claim_code":"AAAA-BBBB"}`)
	}
	reg, err := a.Store.RegistrationByID(id)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	if reg.ClaimCode == code {
		t.Fatal("code should have been force-rotated after repeated wrong guesses")
	}
}

// TestHeartbeatLifecycle walks the client's single loop across the claim: the
// SAME token must keep working, which is what lets the client transition without
// re-keying.
func TestHeartbeatLifecycle(t *testing.T) {
	a, acc := enrollFixture(t)
	id, token, code := register(t, a)

	beat := func(tok string) (*httptest.ResponseRecorder, map[string]any) {
		r := httptest.NewRequest("POST", "/api/devices/self/heartbeat", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		a.deviceHeartbeat(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w, out
	}

	w, out := beat(token)
	if w.Code != http.StatusOK || out["claimed"] != false {
		t.Fatalf("unclaimed heartbeat: %d %v", w.Code, out)
	}
	if out["claim_code"] != code {
		t.Fatalf("heartbeat should echo the current code, got %v", out["claim_code"])
	}

	// Claim it, then heartbeat again with the very same token.
	if _, err := a.Store.ClaimRegistration(id, code, acc.ID, "box", 0); err != nil {
		t.Fatalf("claim: %v", err)
	}
	w, out = beat(token)
	if w.Code != http.StatusOK || out["claimed"] != true {
		t.Fatalf("claimed heartbeat: %d %v", w.Code, out)
	}
	// The disclosure that makes a shoulder-surf claim visible to the victim.
	if out["claimed_by"] != acc.Email {
		t.Fatalf("heartbeat must disclose the claiming account, got %v", out["claimed_by"])
	}

	// An unknown token is a dead credential: the client wipes and re-registers.
	if w, _ := beat("abd_dev_bogus"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown token should be 404, got %d", w.Code)
	}
	if w := httptest.NewRecorder(); true {
		a.deviceHeartbeat(w, httptest.NewRequest("POST", "/api/devices/self/heartbeat", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("missing token should be 401, got %d", w.Code)
		}
	}
}

// TestUnclaimedDeviceIsInvisible is the isolation guarantee: while unclaimed, the
// device must be unreachable through every account-scoped path, and must not
// resolve as a relay credential. This is what makes "an unowned device cannot be
// commanded" structural rather than a policy check.
func TestUnclaimedDeviceIsInvisible(t *testing.T) {
	a, acc := enrollFixture(t)
	id, token, _ := register(t, a)

	if _, err := a.Store.DeviceOwnedBy(id, acc.ID); err != store.ErrNotFound {
		t.Fatalf("unclaimed device must not be owned, got %v", err)
	}
	// The /device relay resolver authenticates via DeviceByTokenHash — an
	// unclaimed device must not pass it, so it cannot join the hub.
	if _, err := a.Store.DeviceByTokenHash(auth.HashToken(token)); err != store.ErrNotFound {
		t.Fatalf("unclaimed token must not resolve for the relay, got %v", err)
	}
	// And it must not resolve by id either (the browser-device Host path).
	if _, err := a.Store.DeviceByID(id); err != store.ErrNotFound {
		t.Fatalf("unclaimed device must not resolve by id, got %v", err)
	}

	devs, err := a.Store.DevicesByAccount(acc.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devs) != 0 {
		t.Fatalf("unclaimed device must not be listed, got %d", len(devs))
	}
}

// TestRegistrationThrottled: the anonymous create endpoint sheds load past its
// burst and says when to come back.
func TestRegistrationThrottled(t *testing.T) {
	a, _ := enrollFixture(t)
	var last *httptest.ResponseRecorder
	for i := 0; i < 5; i++ {
		last, _ = postJSON(t, a.registerDevice, "/api/devices/register", `{"platform":"linux"}`)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 past burst, got %d", last.Code)
	}
	if last.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

// TestClaimCodeNormalization: a human retyping the code should not have to match
// case or punctuation, matching how /pair already treats user codes.
func TestClaimCodeNormalization(t *testing.T) {
	a, acc := enrollFixture(t)
	id, _, code := register(t, a)

	sloppy := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	body := `{"device_id":"` + id + `","claim_code":"` + sloppy + `","accepted":true}`
	r := withAccount(httptest.NewRequest("POST", "/api/claim", strings.NewReader(body)), acc)
	w := httptest.NewRecorder()
	a.claimDevice(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("sloppy code should be accepted, got %d (%s)", w.Code, w.Body.String())
	}
}
