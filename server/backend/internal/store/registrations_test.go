package store

import (
	"path/filepath"
	"testing"
	"time"

	"abacad/internal/auth"
)

const testCodeTTL = 5 * time.Minute

// TestClaimGraduatesIdentity is the central guarantee of the self-enrollment
// design: claiming moves a registration into `devices` WITHOUT changing the id or
// the token. The id is what the human read off the device's screen, and the token
// is what the client already has in its keychain — if either changed, the client
// would have to re-key mid-flow and the id in the URL would not match the screen.
func TestClaimGraduatesIdentity(t *testing.T) {
	s := openTemp(t)
	acc, err := s.CreateAccount("a@b.com", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}

	reg, token, err := s.CreateRegistration("macos", "Ana's laptop", "0.4.0", "1.2.3.4", testCodeTTL)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Before the claim the device is invisible to every account-scoped read.
	if _, err := s.DeviceByID(reg.ID); err != ErrNotFound {
		t.Fatalf("unclaimed device must not resolve by id, got %v", err)
	}
	if _, err := s.DeviceByTokenHash(auth.HashToken(token)); err != ErrNotFound {
		t.Fatalf("unclaimed device must not resolve by token, got %v", err)
	}
	if _, err := s.DeviceOwnedBy(reg.ID, acc.ID); err != ErrNotFound {
		t.Fatalf("unclaimed device must not be owned, got %v", err)
	}

	d, err := s.ClaimRegistration(reg.ID, reg.ClaimCode, acc.ID, "Ana's laptop", 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	if d.ID != reg.ID {
		t.Fatalf("device id changed across claim: registered %q, claimed %q", reg.ID, d.ID)
	}
	if d.Platform != "macos" {
		t.Fatalf("platform lost across claim: %q", d.Platform)
	}
	if d.Version != "0.4.0" {
		t.Fatalf("version lost across claim: %q", d.Version)
	}
	// The same token the client already holds must now resolve as a real device.
	got, err := s.DeviceByTokenHash(auth.HashToken(token))
	if err != nil {
		t.Fatalf("original token must resolve after claim: %v", err)
	}
	if got.ID != reg.ID || got.AccountID != acc.ID {
		t.Fatalf("token resolved to wrong device: %+v", got)
	}
	// And the registration is gone.
	if _, err := s.RegistrationByID(reg.ID); err != ErrNotFound {
		t.Fatalf("registration should be deleted after claim, got %v", err)
	}
}

// TestNewDevicesHumanizeOff guards a default that is easy to lose: the humanize
// column's DEFAULT is 1 (migration 0009), and 0010 flipped the product default to
// off only via a one-time UPDATE. Any INSERT that omits the column therefore
// enrols a device with humanize ON, bypassing the attestation gate. Both device
// creation paths must write it explicitly.
//
// The bug this pins was live and invisible: migration 0010 re-runs on every boot,
// so a wrongly-defaulted device silently corrected itself at the next restart.
func TestNewDevicesHumanizeOff(t *testing.T) {
	s := openTemp(t)
	acc, _ := s.CreateAccount("a@b.com", "hash")

	direct, _, err := s.CreateDevice(acc.ID, "direct", "android", 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.DeviceOwnedBy(direct.ID, acc.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Humanize {
		t.Error("CreateDevice must persist humanize=off; it requires per-device attestation")
	}

	reg, _, _ := s.CreateRegistration("linux", "claimed", "", "", testCodeTTL)
	claimed, err := s.ClaimRegistration(reg.ID, reg.ClaimCode, acc.ID, "claimed", 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	got, err = s.DeviceOwnedBy(claimed.ID, acc.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Humanize {
		t.Error("ClaimRegistration must persist humanize=off")
	}
}

// TestClaimRejects covers every way a claim must fail, including the race: two
// concurrent claims of the same registration must not both graduate it.
func TestClaimRejects(t *testing.T) {
	s := openTemp(t)
	acc, _ := s.CreateAccount("a@b.com", "hash")
	other, _ := s.CreateAccount("b@b.com", "hash")

	t.Run("wrong code", func(t *testing.T) {
		reg, _, _ := s.CreateRegistration("linux", "box", "", "", testCodeTTL)
		if _, err := s.ClaimRegistration(reg.ID, "AAAA-BBBB", acc.ID, "x", 0); err != ErrNotFound {
			t.Fatalf("wrong code should be ErrNotFound, got %v", err)
		}
		// The registration survives a wrong guess — otherwise a typo would
		// destroy the device's identity.
		if _, err := s.RegistrationByID(reg.ID); err != nil {
			t.Fatalf("registration should survive a wrong code: %v", err)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		if _, err := s.ClaimRegistration("nosuchdeviceidxx", "AAAA-BBBB", acc.ID, "x", 0); err != ErrNotFound {
			t.Fatalf("unknown id should be ErrNotFound, got %v", err)
		}
	})

	t.Run("expired code", func(t *testing.T) {
		reg, _, _ := s.CreateRegistration("linux", "box", "", "", -time.Minute) // already expired
		if _, err := s.ClaimRegistration(reg.ID, reg.ClaimCode, acc.ID, "x", 0); err != ErrNotFound {
			t.Fatalf("expired code should be ErrNotFound, got %v", err)
		}
	})

	t.Run("double claim", func(t *testing.T) {
		reg, _, _ := s.CreateRegistration("linux", "box", "", "", testCodeTTL)
		if _, err := s.ClaimRegistration(reg.ID, reg.ClaimCode, acc.ID, "first", 0); err != nil {
			t.Fatalf("first claim: %v", err)
		}
		// A second claim with the same code must not graduate anything, and must
		// not hand the device to a different account.
		if _, err := s.ClaimRegistration(reg.ID, reg.ClaimCode, other.ID, "second", 0); err != ErrNotFound {
			t.Fatalf("second claim should be ErrNotFound, got %v", err)
		}
		d, err := s.DeviceOwnedBy(reg.ID, acc.ID)
		if err != nil {
			t.Fatalf("first claimer should still own it: %v", err)
		}
		if d.Name != "first" {
			t.Fatalf("second claim overwrote the name: %q", d.Name)
		}
		if _, err := s.DeviceOwnedBy(reg.ID, other.ID); err != ErrNotFound {
			t.Fatalf("second account must not own the device, got %v", err)
		}
	})
}

// TestClaimCodeRotation checks that rotation issues a new code, invalidates the
// old one, and resets the failed-attempt counter.
func TestClaimCodeRotation(t *testing.T) {
	s := openTemp(t)
	acc, _ := s.CreateAccount("a@b.com", "hash")
	reg, _, _ := s.CreateRegistration("windows", "pc", "", "", testCodeTTL)

	n, err := s.NoteClaimAttempt(reg.ID)
	if err != nil || n != 1 {
		t.Fatalf("attempt count: got %d, err %v", n, err)
	}

	rotated, err := s.RotateClaimCode(reg.ID, testCodeTTL)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if rotated.ClaimCode == reg.ClaimCode {
		t.Fatal("rotation returned the same code")
	}
	if rotated.ClaimAttempts != 0 {
		t.Fatalf("rotation should reset attempts, got %d", rotated.ClaimAttempts)
	}
	// The pre-rotation code must be dead.
	if _, err := s.ClaimRegistration(reg.ID, reg.ClaimCode, acc.ID, "x", 0); err != ErrNotFound {
		t.Fatalf("stale code should be ErrNotFound, got %v", err)
	}
	if _, err := s.ClaimRegistration(reg.ID, rotated.ClaimCode, acc.ID, "x", 0); err != nil {
		t.Fatalf("rotated code should work: %v", err)
	}
}

// TestRegistrationGC covers both reap cutoffs independently — idle is what
// catches a spammer (rows that never heartbeat), while the absolute cap bounds a
// client that idles on its setup screen forever.
func TestRegistrationGC(t *testing.T) {
	s := openTemp(t)
	now := time.Now().Unix()

	live, _, _ := s.CreateRegistration("linux", "live", "", "", testCodeTTL)
	idle, _, _ := s.CreateRegistration("linux", "idle", "", "", testCodeTTL)
	old, _, _ := s.CreateRegistration("linux", "old", "", "", testCodeTTL)

	// Backdate: `idle` stopped heartbeating; `old` still heartbeats but was
	// registered long ago.
	if _, err := s.db.Exec(`UPDATE device_registrations SET last_seen=? WHERE id=?`, now-7200, idle.ID); err != nil {
		t.Fatalf("backdate idle: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE device_registrations SET created_at=?, last_seen=? WHERE id=?`,
		now-30*86400, now, old.ID); err != nil {
		t.Fatalf("backdate old: %v", err)
	}

	n, err := s.DeleteStaleRegistrations(now-3600, now-7*86400)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 reaped, got %d", n)
	}
	if _, err := s.RegistrationByID(live.ID); err != nil {
		t.Fatalf("live registration must survive GC: %v", err)
	}
	for _, id := range []string{idle.ID, old.ID} {
		if _, err := s.RegistrationByID(id); err != ErrNotFound {
			t.Fatalf("stale registration %s should be reaped, got %v", id, err)
		}
	}
}

// TestPairingGC covers the drive-by fix: device_pairings had no GC at all, so
// every `abacad connect` leaked a row forever.
func TestPairingGC(t *testing.T) {
	s := openTemp(t)
	if _, _, err := s.CreatePairing("linux", time.Hour); err != nil {
		t.Fatalf("fresh pairing: %v", err)
	}
	if _, _, err := s.CreatePairing("linux", -time.Hour); err != nil { // already expired
		t.Fatalf("stale pairing: %v", err)
	}
	n, err := s.DeletePairingsExpiredBefore(time.Now().Unix())
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 pairing reaped, got %d", n)
	}
}

// TestRegistrationQuotaCounters backs the per-IP and global caps.
func TestRegistrationQuotaCounters(t *testing.T) {
	s := openTemp(t)
	now := time.Now().Unix()
	for i := 0; i < 3; i++ {
		if _, _, err := s.CreateRegistration("linux", "x", "", "9.9.9.9", testCodeTTL); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	if _, _, err := s.CreateRegistration("linux", "x", "", "8.8.8.8", testCodeTTL); err != nil {
		t.Fatalf("register: %v", err)
	}

	n, err := s.LiveRegistrationsByIP("9.9.9.9", now-60)
	if err != nil || n != 3 {
		t.Fatalf("per-ip count: got %d, err %v", n, err)
	}
	total, err := s.CountRegistrations()
	if err != nil || total != 4 {
		t.Fatalf("global count: got %d, err %v", total, err)
	}
	// Rows that stopped heartbeating don't count toward the concurrent cap.
	if n, _ := s.LiveRegistrationsByIP("9.9.9.9", now+60); n != 0 {
		t.Fatalf("stale rows should not count as live, got %d", n)
	}
}

// TestMigrationIdempotent is the contract migrate() promises: opening the same
// database twice must be clean and must not disturb existing rows. The ledger
// means the second Open() skips every file, but the migrations stay individually
// idempotent (0012 is CREATE TABLE IF NOT EXISTS) so the pre-ledger catch-up
// pass is also safe.
func TestMigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	reg, _, err := s1.CreateRegistration("linux", "box", "", "", testCodeTTL)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second open (migrations not idempotent): %v", err)
	}
	defer s2.Close()
	// Re-running migrations must not have dropped the data.
	if _, err := s2.RegistrationByID(reg.ID); err != nil {
		t.Fatalf("registration lost across reopen: %v", err)
	}
}

// TestMigrationsDoNotResetDeviceState guards the failure the schema_migrations
// ledger exists to prevent. 0010_humanize_default_off.sql carries a bare
// `UPDATE devices SET humanize = 0` meant as a one-time reset; without a ledger
// it re-ran on every Open(), silently clearing a flag the operator had to pass
// an attestation gate to enable. Restarting the server must never revoke an
// operator's explicit choice.
func TestMigrationsDoNotResetDeviceState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	acc, err := s1.CreateAccount("a@b.com", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	dev, _, err := s1.CreateDevice(acc.ID, "phone", "android", 0)
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	if err := s1.SetDeviceHumanize(dev.ID, acc.ID, true); err != nil {
		t.Fatalf("set humanize: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()
	got, err := s2.DeviceByID(dev.ID)
	if err != nil {
		t.Fatalf("device lost across reopen: %v", err)
	}
	if !got.Humanize {
		t.Fatal("humanize was reset by re-running migrations on restart")
	}
}
