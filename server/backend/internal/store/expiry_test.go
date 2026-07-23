package store

import (
	"testing"
	"time"

	"abacad/internal/auth"
)

// TestDeviceExpiry covers the enrollment-expiry gate: expired devices don't
// resolve for connect but stay visible to their owner, and the sweep helpers
// select/reap the right rows.
func TestDeviceExpiry(t *testing.T) {
	s := openTemp(t)
	a, err := s.CreateAccount("a@b.com", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	now := time.Now().Unix()

	_, permTok, _ := s.CreateDevice(a.ID, "perm", "android", 0)          // permanent
	_, futTok, _ := s.CreateDevice(a.ID, "future", "android", now+3600)  // valid
	expDev, expTok, _ := s.CreateDevice(a.ID, "old", "android", now-1)   // expired

	// Connect-time lookup: permanent and future resolve, expired does not.
	if _, err := s.DeviceByTokenHash(auth.HashToken(permTok)); err != nil {
		t.Fatalf("permanent should resolve: %v", err)
	}
	if _, err := s.DeviceByTokenHash(auth.HashToken(futTok)); err != nil {
		t.Fatalf("future should resolve: %v", err)
	}
	if _, err := s.DeviceByTokenHash(auth.HashToken(expTok)); err != ErrNotFound {
		t.Fatalf("expired should be ErrNotFound, got %v", err)
	}

	// Owner management still sees the expired (dormant) device.
	if _, err := s.DeviceOwnedBy(expDev.ID, a.ID); err != nil {
		t.Fatalf("owner should still see expired device: %v", err)
	}

	// ExpiredDeviceIDs returns only the expired one (never the permanent).
	ids, err := s.ExpiredDeviceIDs(now)
	if err != nil {
		t.Fatalf("ExpiredDeviceIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != expDev.ID {
		t.Fatalf("ExpiredDeviceIDs = %v, want [%s]", ids, expDev.ID)
	}

	// Extend the expired device forward; it resolves again and drops off the list.
	if err := s.SetDeviceExpiry(expDev.ID, a.ID, now+3600); err != nil {
		t.Fatalf("extend: %v", err)
	}
	if _, err := s.DeviceByTokenHash(auth.HashToken(expTok)); err != nil {
		t.Fatalf("extended device should resolve: %v", err)
	}

	// Re-expire it and reap past the grace cutoff; permanent + future survive.
	_ = s.SetDeviceExpiry(expDev.ID, a.ID, now-100)
	n, err := s.DeleteDevicesExpiredBefore(now)
	if err != nil || n != 1 {
		t.Fatalf("DeleteDevicesExpiredBefore n=%d err=%v, want 1/nil", n, err)
	}
	if _, err := s.DeviceOwnedBy(expDev.ID, a.ID); err != ErrNotFound {
		t.Fatalf("reaped device should be gone, got %v", err)
	}
	if _, err := s.DeviceByTokenHash(auth.HashToken(permTok)); err != nil {
		t.Fatalf("permanent must survive reap: %v", err)
	}
}
