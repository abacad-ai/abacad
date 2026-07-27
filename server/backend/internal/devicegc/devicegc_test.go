package devicegc

import (
	"path/filepath"
	"testing"
	"time"

	"abacad/internal/relay"
	"abacad/internal/store"
)

// TestSweepReapsPastGrace: a sweep deletes devices expired longer than the grace
// window and keeps ones still within it (dormant). The kick step is a no-op here
// (no live connections); Hub.Kick is covered in the relay package.
func TestSweepReapsPastGrace(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	acc, err := st.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	now := time.Now().Unix()

	// Past grace (expired 2h ago, grace 1h) -> reaped.
	old, _, _ := st.CreateDevice(acc.ID, "old", "android", now-2*3600)
	// Within grace (expired 10s ago) -> kicked but kept dormant.
	recent, _, _ := st.CreateDevice(acc.ID, "recent", "android", now-10)
	// Permanent -> untouched.
	perm, _, _ := st.CreateDevice(acc.ID, "perm", "android", 0)

	s := &Sweeper{st: st, hub: relay.NewHub(), grace: time.Hour}
	s.sweep()

	if _, err := st.DeviceOwnedBy(old.ID, acc.ID); err != store.ErrNotFound {
		t.Fatalf("device past grace should be reaped, got %v", err)
	}
	if _, err := st.DeviceOwnedBy(recent.ID, acc.ID); err != nil {
		t.Fatalf("device within grace should remain dormant: %v", err)
	}
	if _, err := st.DeviceOwnedBy(perm.ID, acc.ID); err != nil {
		t.Fatalf("permanent device must survive: %v", err)
	}
}

// TestSweepReapsPreEnrollment covers steps 3 and 4: unclaimed registrations that
// stopped heartbeating are reaped while live ones keep their id, and expired
// pairings are cleaned up.
func TestSweepReapsPreEnrollment(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	live, _, err := st.CreateRegistration("linux", "live", "", "", 5*time.Minute)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	idle, _, _ := st.CreateRegistration("linux", "idle", "", "", 5*time.Minute)
	// Stop `idle` heartbeating well past registrationIdleTTL.
	if _, err := st.TouchRegistrationAt(idle.ID, time.Now().Add(-3*time.Hour).Unix()); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if _, _, err := st.CreatePairing("linux", -48*time.Hour); err != nil { // long expired
		t.Fatalf("stale pairing: %v", err)
	}
	if _, _, err := st.CreatePairing("linux", time.Hour); err != nil { // in flight
		t.Fatalf("fresh pairing: %v", err)
	}

	s := &Sweeper{st: st, hub: relay.NewHub(), grace: time.Hour}
	s.sweep()

	if _, err := st.RegistrationByID(live.ID); err != nil {
		t.Fatalf("heartbeating registration must keep its id: %v", err)
	}
	if _, err := st.RegistrationByID(idle.ID); err != store.ErrNotFound {
		t.Fatalf("idle registration should be reaped, got %v", err)
	}

	n, err := st.CountPairings()
	if err != nil {
		t.Fatalf("count pairings: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected only the in-flight pairing to survive, got %d", n)
	}
}
