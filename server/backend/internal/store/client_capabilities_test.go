package store

import (
	"testing"

	"abacad/internal/protocol"
)

func newDeviceForCaps(t *testing.T) (*Store, Device) {
	t.Helper()
	s := openTemp(t)
	acc, err := s.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	d, _, err := s.CreateDevice(acc.ID, "box", "linux", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	return s, d
}

// A client that has never reported imposes no ceiling. Clients predating the
// capabilities frame never send one, and reading their silence as "expose
// nothing" would take every existing device offline on upgrade.
func TestUnreportedClientImposesNoCeiling(t *testing.T) {
	s, d := newDeviceForCaps(t)
	eff, err := s.EffectiveDeviceCapabilities(d.ID)
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	for _, c := range protocol.Capabilities {
		if !eff.Allows(c) {
			t.Errorf("%s should be exposed by an unreported device", c)
		}
	}
}

// The effective set is the intersection: either side may narrow, neither may
// widen. This is what makes the device the grounded authority — the server
// agreeing is not what permits the action, and the server disagreeing cannot
// re-enable it.
func TestEffectiveIsIntersection(t *testing.T) {
	s, d := newDeviceForCaps(t)
	acc := d.AccountID

	// Device says: screenshot and click only.
	if err := s.SetClientCapabilities(d.ID, protocol.NewCapabilitySet(
		protocol.Capability(protocol.MethodScreenshot),
		protocol.Capability(protocol.MethodClick),
	)); err != nil {
		t.Fatalf("set client caps: %v", err)
	}
	// Account says: click and push_file only.
	if err := s.SetDeviceCapabilities(d.ID, acc, protocol.NewCapabilitySet(
		protocol.Capability(protocol.MethodClick),
		protocol.Capability(protocol.MethodPushFile),
	)); err != nil {
		t.Fatalf("set account caps: %v", err)
	}

	eff, err := s.EffectiveDeviceCapabilities(d.ID)
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	if !eff.Allows(protocol.Capability(protocol.MethodClick)) {
		t.Error("click is allowed by both, so it must be effective")
	}
	// Allowed by the device but not the account.
	if eff.Allows(protocol.Capability(protocol.MethodScreenshot)) {
		t.Error("the account did not grant screenshot")
	}
	// Granted by the account but refused by the device — the case that matters:
	// the server cannot widen past what the device declared.
	if eff.Allows(protocol.Capability(protocol.MethodPushFile)) {
		t.Error("the account must not be able to grant what the device refused")
	}
}

// A device reporting an empty list is asking to expose nothing, which is
// different from never reporting at all.
func TestClientCanRefuseEverything(t *testing.T) {
	s, d := newDeviceForCaps(t)
	if err := s.SetClientCapabilities(d.ID, protocol.NewCapabilitySet()); err != nil {
		t.Fatalf("set client caps: %v", err)
	}
	eff, err := s.EffectiveDeviceCapabilities(d.ID)
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	if got := eff.List(); len(got) != 0 {
		t.Fatalf("expected nothing exposed, got %v", got)
	}
}

// The wildcard must survive the round trip as a wildcard, not be flattened into
// the current verb list — otherwise a device pins itself to the capabilities its
// version knew about and silently refuses anything added later.
func TestClientWildcardRoundTrips(t *testing.T) {
	s, d := newDeviceForCaps(t)
	if err := s.SetClientCapabilities(d.ID, protocol.AllCapabilities()); err != nil {
		t.Fatalf("set client caps: %v", err)
	}
	got, err := s.DeviceByID(d.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !got.ClientCapabilities.All() {
		t.Fatal("wildcard did not survive the round trip")
	}
}
