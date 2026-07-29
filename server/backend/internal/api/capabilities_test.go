package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"abacad/internal/protocol"
	"abacad/internal/relay"
	"abacad/internal/store"
)

// patchDevice PATCHes a device as its owner and returns the status code.
func patchDevice(t *testing.T, a *API, acc store.Account, devID, body string) int {
	t.Helper()
	r := httptest.NewRequest("PATCH", "/api/devices/"+devID, strings.NewReader(body))
	r.SetPathValue("id", devID)
	r = r.WithContext(context.WithValue(r.Context(), accountKey, acc))
	w := httptest.NewRecorder()
	a.updateDevice(w, r)
	return w.Code
}

// A newly enrolled device exposes everything: enrollment is the authorization,
// so this config narrows an already-trusted device rather than gating a new one.
// Shipping it default-closed would break every device on upgrade.
func TestNewDeviceExposesEverything(t *testing.T) {
	a, acc := gateFixture(t)
	dev, _, err := a.Store.CreateDevice(acc.ID, "phone", "android", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	caps, err := a.Store.DeviceCapabilities(dev.ID)
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	for _, c := range protocol.Capabilities {
		if !caps.Allows(c) {
			t.Errorf("new device does not expose %s", c)
		}
	}
}

func TestSetCapabilitiesRoundTrip(t *testing.T) {
	a, acc := gateFixture(t)
	a.Hub = relay.NewHub(relay.AllowAllGate)
	dev, _, err := a.Store.CreateDevice(acc.ID, "laptop", "macos", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}

	// An observe-only device: it may be looked at, never typed into.
	if code := patchDevice(t, a, acc, dev.ID, `{"capabilities":["screenshot"]}`); code != 204 {
		t.Fatalf("set capabilities: got %d, want 204", code)
	}
	caps, err := a.Store.DeviceCapabilities(dev.ID)
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !caps.Allows(protocol.Capability(protocol.MethodScreenshot)) {
		t.Error("screenshot should be exposed")
	}
	for _, denied := range []protocol.Capability{
		protocol.Capability(protocol.MethodClick),
		protocol.Capability(protocol.MethodPushFile),
		protocol.CapTunnel,
		protocol.CapSSH,
		protocol.CapVNC,
	} {
		if caps.Allows(denied) {
			t.Errorf("%s should not be exposed", denied)
		}
	}

	// The view exposes the concrete list, never the raw wildcard.
	v := a.viewDevice(mustDevice(t, a, dev.ID))
	if len(v.Capabilities) != 1 || v.Capabilities[0] != "screenshot" {
		t.Fatalf("view capabilities = %v", v.Capabilities)
	}

	// An explicit empty list is meaningful: expose nothing at all.
	if code := patchDevice(t, a, acc, dev.ID, `{"capabilities":[]}`); code != 204 {
		t.Fatalf("clear capabilities: got %d, want 204", code)
	}
	caps, _ = a.Store.DeviceCapabilities(dev.ID)
	if len(caps.List()) != 0 {
		t.Errorf("expected no capabilities, got %v", caps.List())
	}

	// Omitting the key entirely must leave the setting untouched — the handler
	// is a partial update, and a rename must not silently re-open everything.
	if code := patchDevice(t, a, acc, dev.ID, `{"name":"renamed"}`); code != 204 {
		t.Fatalf("rename: got %d, want 204", code)
	}
	caps, _ = a.Store.DeviceCapabilities(dev.ID)
	if len(caps.List()) != 0 {
		t.Errorf("rename changed capabilities to %v", caps.List())
	}
}

// A typo must be rejected rather than stored as a capability that silently never
// matches anything.
func TestUnknownCapabilityRejected(t *testing.T) {
	a, acc := gateFixture(t)
	dev, _, err := a.Store.CreateDevice(acc.ID, "laptop", "macos", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	if code := patchDevice(t, a, acc, dev.ID, `{"capabilities":["screenshot","screenshto"]}`); code != 400 {
		t.Fatalf("unknown capability: got %d, want 400", code)
	}
	// The whole update is rejected, so the original set is intact.
	caps, _ := a.Store.DeviceCapabilities(dev.ID)
	if !caps.All() {
		t.Errorf("a rejected update must not partially apply, got %v", caps.List())
	}
}

// Changing capabilities is a consent-grade event: it must land on the trail so
// the owner can see when their device's exposure changed.
func TestCapabilityChangeRecorded(t *testing.T) {
	a, acc := gateFixture(t)
	a.Hub = relay.NewHub(relay.AllowAllGate)
	dev, _, err := a.Store.CreateDevice(acc.ID, "laptop", "macos", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	if code := patchDevice(t, a, acc, dev.ID, `{"capabilities":["screenshot"]}`); code != 204 {
		t.Fatalf("set: got %d", code)
	}
	if !hasConsent(t, a.Store, acc.ID, "capabilities.set") {
		t.Fatal("no consent activity recorded for capabilities.set")
	}
}

func mustDevice(t *testing.T, a *API, id string) store.Device {
	t.Helper()
	dev, err := a.Store.DeviceByID(id)
	if err != nil {
		t.Fatalf("device %s: %v", id, err)
	}
	return dev
}
