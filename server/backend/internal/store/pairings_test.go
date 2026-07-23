package store

import (
	"strings"
	"testing"
	"time"

	"abacad/internal/auth"
)

func TestPairingLifecycle(t *testing.T) {
	s := openTemp(t)
	acc, _ := s.CreateAccount("p@q.com", "h")

	deviceCode, userCode, err := s.CreatePairing("linux", 10*time.Minute)
	if err != nil {
		t.Fatalf("create pairing: %v", err)
	}
	if !strings.HasPrefix(deviceCode, "abd_pair_") {
		t.Fatalf("device_code prefix: %q", deviceCode)
	}
	if len(userCode) != 9 || userCode[4] != '-' {
		t.Fatalf("user_code format: %q", userCode)
	}

	// Before approval, the CLI poll sees a pending row.
	p, err := s.PairingByDeviceCode(deviceCode)
	if err != nil || p.Status != PairingPending || p.Consumed {
		t.Fatalf("pending lookup: %v %+v", err, p)
	}

	// Consuming a not-yet-approved pairing must not mint a device.
	if _, _, err := s.ConsumePairing(deviceCode, 0); err != ErrNotFound {
		t.Fatalf("consume-before-approve want ErrNotFound, got %v", err)
	}

	// Human approves. An empty platform override must PRESERVE the platform the
	// CLI reported at start ("linux") — not blank it out.
	if err := s.ApprovePairing(userCode, acc.ID, "My box", ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// The CLI's next poll consumes it and mints exactly one device.
	d, token, err := s.ConsumePairing(deviceCode, 0)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if d.AccountID != acc.ID || d.Name != "My box" || d.Platform != "linux" {
		t.Fatalf("minted device fields: %+v", d)
	}
	// The returned token must actually resolve to the new device.
	if got, err := s.DeviceByTokenHash(auth.HashToken(token)); err != nil || got.ID != d.ID {
		t.Fatalf("token does not resolve: %v %+v", err, got)
	}

	// A second poll must not mint a second device (idempotency / race guard).
	if _, _, err := s.ConsumePairing(deviceCode, 0); err != ErrNotFound {
		t.Fatalf("double-consume want ErrNotFound, got %v", err)
	}
	if ds, _ := s.DevicesByAccount(acc.ID); len(ds) != 1 {
		t.Fatalf("want exactly 1 device, got %d", len(ds))
	}
}

func TestPairingApproveRejects(t *testing.T) {
	s := openTemp(t)
	acc, _ := s.CreateAccount("r@s.com", "h")

	// Unknown code.
	if err := s.ApprovePairing("ZZZZ-9999", acc.ID, "n", "linux"); err != ErrNotFound {
		t.Fatalf("unknown code want ErrNotFound, got %v", err)
	}

	// Expired pairing is neither approvable nor consumable.
	dc, uc, _ := s.CreatePairing("linux", -time.Minute)
	if err := s.ApprovePairing(uc, acc.ID, "n", "linux"); err != ErrNotFound {
		t.Fatalf("expired approve want ErrNotFound, got %v", err)
	}
	if _, _, err := s.ConsumePairing(dc, 0); err != ErrNotFound {
		t.Fatalf("expired consume want ErrNotFound, got %v", err)
	}
}

func TestNewUserCodeShape(t *testing.T) {
	// Mirror of the ambiguity-free alphabet in auth.NewUserCode (no 0/O, 1/I/L).
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		c := auth.NewUserCode()
		if len(c) != 9 || c[4] != '-' {
			t.Fatalf("bad shape: %q", c)
		}
		for j, r := range c {
			if j == 4 {
				continue
			}
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("char %q not in alphabet (code %q)", r, c)
			}
		}
		seen[c] = true
	}
	if len(seen) < 490 { // essentially all unique
		t.Fatalf("suspicious collision rate: %d unique of 500", len(seen))
	}
}
