package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestExtendAndPermanentGate covers the enrollment PATCH actions: extend moves
// expiry forward; make-permanent requires attestation (422 otherwise) and records
// a consent row; and extend is rejected when the server runs with no TTL.
func TestExtendAndPermanentGate(t *testing.T) {
	a, acc := gateFixture(t)
	a.EnrollmentTTL = time.Hour // hosted-style: enrollment expires

	dev, _, err := a.Store.CreateDevice(acc.ID, "phone", "android", a.enrollmentExpiry())
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

	// Extend moves expiry forward (still non-zero).
	before, _ := a.Store.DeviceOwnedBy(dev.ID, acc.ID)
	if code := call(`{"extend":true}`); code != 204 {
		t.Fatalf("extend: got %d, want 204", code)
	}
	after, _ := a.Store.DeviceOwnedBy(dev.ID, acc.ID)
	if after.ExpiresAt < before.ExpiresAt || after.ExpiresAt == 0 {
		t.Fatalf("extend did not move expiry forward: before=%d after=%d", before.ExpiresAt, after.ExpiresAt)
	}

	// Make permanent without attestation is rejected.
	if code := call(`{"permanent":true}`); code != 422 {
		t.Fatalf("permanent without attestation: got %d, want 422", code)
	}
	// With attestation: expiry cleared + consent recorded.
	if code := call(`{"permanent":true,"attested":true}`); code != 204 {
		t.Fatalf("permanent with attestation: got %d, want 204", code)
	}
	if d, _ := a.Store.DeviceOwnedBy(dev.ID, acc.ID); d.ExpiresAt != 0 {
		t.Fatalf("permanent should clear expiry, got %d", d.ExpiresAt)
	}
	if !hasConsent(t, a.Store, acc.ID, "enrollment.permanent") {
		t.Fatal("no consent activity recorded for enrollment.permanent")
	}
}

// TestExtendRejectedWithoutTTL: a self-hosted instance (no TTL) has nothing to
// extend, so extend returns 400.
func TestExtendRejectedWithoutTTL(t *testing.T) {
	a, acc := gateFixture(t) // EnrollmentTTL defaults to 0
	dev, _, _ := a.Store.CreateDevice(acc.ID, "phone", "android", 0)

	r := httptest.NewRequest("PATCH", "/api/devices/"+dev.ID, strings.NewReader(`{"extend":true}`))
	r.SetPathValue("id", dev.ID)
	r = r.WithContext(context.WithValue(r.Context(), accountKey, acc))
	w := httptest.NewRecorder()
	a.updateDevice(w, r)
	if w.Code != 400 {
		t.Fatalf("extend with no TTL: got %d, want 400", w.Code)
	}
}
