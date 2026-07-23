package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"abacad/internal/activity"
	"abacad/internal/store"
)

// gateFixture builds an API over a temp store with a real (async) activity
// recorder and returns it with one account.
func gateFixture(t *testing.T) (*API, store.Account) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	acc, err := st.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	return &API{Store: st, Activity: activity.New(st, 0)}, acc
}

// hasConsent polls the trail for a consent row with the given method.
func hasConsent(t *testing.T, st *store.Store, accID, method string) bool {
	t.Helper()
	for i := 0; i < 50; i++ {
		acts, err := st.Activities(accID, store.ActivityFilter{Kind: activity.KindConsent})
		if err != nil {
			t.Fatalf("activities: %v", err)
		}
		for _, a := range acts {
			if a.Method == method {
				return true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// TestHumanizeAttestationGate: enabling humanize requires attested=true (422
// otherwise) and records a consent row; disabling never needs attestation.
func TestHumanizeAttestationGate(t *testing.T) {
	a, acc := gateFixture(t)
	dev, _, err := a.Store.CreateDevice(acc.ID, "phone", "android", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}

	call := func(body string) int {
		r := httptest.NewRequest("PATCH", "/api/devices/"+dev.ID, strings.NewReader(body))
		r.SetPathValue("id", dev.ID)
		r = r.WithContext(context.WithValue(r.Context(), accountKey, acc))
		w := httptest.NewRecorder()
		a.updateDevice(w, r)
		return w.Code
	}

	if code := call(`{"humanize":true}`); code != 422 {
		t.Fatalf("enable without attestation: got %d, want 422", code)
	}
	if code := call(`{"humanize":true,"attested":true}`); code != 204 {
		t.Fatalf("enable with attestation: got %d, want 204", code)
	}
	if !hasConsent(t, a.Store, acc.ID, "humanize.enable") {
		t.Fatal("no consent activity recorded for humanize.enable")
	}
	// The store must reflect the enabled flag.
	if d, _ := a.Store.DeviceByID(dev.ID); !d.Humanize {
		t.Fatal("humanize not persisted after attested enable")
	}
	// Disabling needs no attestation.
	if code := call(`{"humanize":false}`); code != 204 {
		t.Fatalf("disable: got %d, want 204", code)
	}
	if d, _ := a.Store.DeviceByID(dev.ID); d.Humanize {
		t.Fatal("humanize still on after disable")
	}
}

// TestPairApprovalGate: approving an enrollment requires accepted=true.
func TestPairApprovalGate(t *testing.T) {
	a, acc := gateFixture(t)
	_, userCode, err := a.Store.CreatePairing("android", time.Minute)
	if err != nil {
		t.Fatalf("create pairing: %v", err)
	}

	call := func(body string) int {
		r := httptest.NewRequest("POST", "/api/devices/pair", strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), accountKey, acc))
		w := httptest.NewRecorder()
		a.pairApprove(w, r)
		return w.Code
	}

	if code := call(`{"user_code":"` + userCode + `","name":"phone"}`); code != 422 {
		t.Fatalf("approve without acceptance: got %d, want 422", code)
	}
	if code := call(`{"user_code":"` + userCode + `","name":"phone","accepted":true}`); code != 200 {
		t.Fatalf("approve with acceptance: got %d, want 200", code)
	}
	if !hasConsent(t, a.Store, acc.ID, "enrollment.accepted") {
		t.Fatal("no consent activity recorded for enrollment.accepted")
	}
}
