package store

import (
	"path/filepath"
	"testing"
	"time"

	"abacad/internal/auth"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAccountsAndSessions(t *testing.T) {
	s := openTemp(t)
	a, err := s.CreateAccount("a@b.com", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.CreateAccount("a@b.com", "hash2"); err != ErrEmailTaken {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
	got, err := s.AccountByEmail("a@b.com")
	if err != nil || got.ID != a.ID {
		t.Fatalf("by email: %v %+v", err, got)
	}

	sid, err := s.CreateSession(a.ID, "ua", time.Hour)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if acc, err := s.AccountBySession(sid); err != nil || acc.ID != a.ID {
		t.Fatalf("by session: %v %+v", err, acc)
	}
	// Expired session must not resolve.
	expired, _ := s.CreateSession(a.ID, "ua", -time.Hour)
	if _, err := s.AccountBySession(expired); err != ErrNotFound {
		t.Fatalf("expired want ErrNotFound, got %v", err)
	}
	_ = s.DeleteSession(sid)
	if _, err := s.AccountBySession(sid); err != ErrNotFound {
		t.Fatalf("deleted want ErrNotFound, got %v", err)
	}
}

func TestDevicesAndTokens(t *testing.T) {
	s := openTemp(t)
	a, _ := s.CreateAccount("d@e.com", "h")
	other, _ := s.CreateAccount("x@y.com", "h")

	d1, tok1, err := s.CreateDevice(a.ID, "Pixel", "android", 0)
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	// Token resolves to the device, platform round-trips.
	if got, err := s.DeviceByTokenHash(auth.HashToken(tok1)); err != nil || got.ID != d1.ID || got.Platform != "android" {
		t.Fatalf("by token: %v %+v", err, got)
	}
	// Ownership is enforced.
	if _, err := s.DeviceOwnedBy(d1.ID, other.ID); err != ErrNotFound {
		t.Fatalf("cross-account want ErrNotFound, got %v", err)
	}
	// Rotation invalidates the old token.
	tok2, err := s.RotateDeviceToken(d1.ID, a.ID)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := s.DeviceByTokenHash(auth.HashToken(tok1)); err != ErrNotFound {
		t.Fatalf("old token should be dead, got %v", err)
	}
	if _, err := s.DeviceByTokenHash(auth.HashToken(tok2)); err != nil {
		t.Fatalf("new token should work: %v", err)
	}

	// last_seen ordering: create d2, touch it, expect it first.
	d2, _, _ := s.CreateDevice(a.ID, "Old", "", 0)
	s.TouchDevice(d2.ID)
	list, err := s.DevicesByAccount(a.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v n=%d", err, len(list))
	}
	if list[0].ID != d2.ID {
		t.Fatalf("touched device should sort first, got %s", list[0].ID)
	}

	if err := s.DeleteDevice(d1.ID, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.DeviceOwnedBy(d1.ID, a.ID); err != ErrNotFound {
		t.Fatalf("deleted want ErrNotFound, got %v", err)
	}
}

func TestKeyScopeAllows(t *testing.T) {
	all := KeyScope{AllDevices: true, AllMethods: true, AllowTunnel: true}
	if !all.AllowsDevice("anything") || !all.AllowsMethod("execute") || !all.AllowsTunnel() {
		t.Fatalf("all-wildcards scope should allow everything")
	}
	narrow := KeyScope{DeviceIDs: []string{"dev1"}, Methods: []string{"screenshot"}}
	if !narrow.AllowsDevice("dev1") || narrow.AllowsDevice("dev2") {
		t.Fatalf("device allowlist not enforced")
	}
	if !narrow.AllowsMethod("screenshot") || narrow.AllowsMethod("tap") {
		t.Fatalf("method allowlist not enforced")
	}
	if narrow.AllowsTunnel() {
		t.Fatalf("tunnel should be off by default")
	}
}

func TestAPIKeys(t *testing.T) {
	s := openTemp(t)
	a, _ := s.CreateAccount("m@n.com", "h")
	d1, _, _ := s.CreateDevice(a.ID, "phone", "android", 0)
	d2, _, _ := s.CreateDevice(a.ID, "mac", "macos", 0)

	// A key scoped to one device + one method, no tunnel.
	scope := KeyScope{DeviceIDs: []string{d1.ID}, Methods: []string{"screenshot"}}
	tok, key, err := s.CreateAPIKey(a.ID, "scoped", scope)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if key.Name != "scoped" || key.ID == "" {
		t.Fatalf("unexpected key: %+v", key)
	}

	accID, got, err := s.APIKeyScopeByTokenHash(auth.HashToken(tok))
	if err != nil || accID != a.ID {
		t.Fatalf("resolve: %v acc=%s", err, accID)
	}
	if got.AllDevices || got.AllMethods || got.AllowTunnel {
		t.Fatalf("wildcards should be off: %+v", got)
	}
	if !got.AllowsDevice(d1.ID) || got.AllowsDevice(d2.ID) {
		t.Fatalf("device scope not persisted: %+v", got.DeviceIDs)
	}
	if !got.AllowsMethod("screenshot") || got.AllowsMethod("tap") {
		t.Fatalf("method scope not persisted: %+v", got.Methods)
	}

	// list reflects the key with last_used stamped by the resolve above.
	keys, err := s.APIKeysByAccount(a.ID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("list: %v n=%d", err, len(keys))
	}
	if keys[0].LastUsed == 0 {
		t.Fatalf("last_used should be stamped after resolve")
	}

	// Update to all-devices + all-methods + tunnel (wildcards, incl. future devices).
	wide := KeyScope{AllDevices: true, AllMethods: true, AllowTunnel: true}
	if err := s.UpdateAPIKey(key.ID, a.ID, "wide", wide); err != nil {
		t.Fatalf("update: %v", err)
	}
	_, got2, _ := s.APIKeyScopeByTokenHash(auth.HashToken(tok))
	d3, _, _ := s.CreateDevice(a.ID, "later", "browser", 0) // created AFTER the key
	if !got2.AllDevices || !got2.AllowsDevice(d3.ID) || !got2.AllowsTunnel() {
		t.Fatalf("all-devices wildcard should cover future devices: %+v", got2)
	}

	// A wrong account can't update or delete.
	if err := s.UpdateAPIKey(key.ID, "acc_other", "x", wide); err != ErrNotFound {
		t.Fatalf("cross-account update want ErrNotFound, got %v", err)
	}
	if err := s.DeleteAPIKey(key.ID, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := s.APIKeyScopeByTokenHash(auth.HashToken(tok)); err != ErrNotFound {
		t.Fatalf("deleted key should be dead, got %v", err)
	}
}

func TestLinkGoogleAccount(t *testing.T) {
	s := openTemp(t)

	// New subject with no matching account -> fresh passwordless account.
	acc, created, err := s.LinkGoogleAccount("sub-1", "new@gmail.com")
	if err != nil || !created {
		t.Fatalf("create path: created=%v err=%v", created, err)
	}
	if acc.PasswordHash != "" {
		t.Fatalf("google account should be passwordless, got %q", acc.PasswordHash)
	}

	// Returning user (same subject) -> same account, not created.
	again, created, err := s.LinkGoogleAccount("sub-1", "new@gmail.com")
	if err != nil || created || again.ID != acc.ID {
		t.Fatalf("returning path: id=%s created=%v err=%v", again.ID, created, err)
	}

	// Existing password account with the same email -> linked, not created.
	pw, _ := s.CreateAccount("existing@gmail.com", "bcrypthash")
	linked, created, err := s.LinkGoogleAccount("sub-2", "existing@gmail.com")
	if err != nil || created || linked.ID != pw.ID {
		t.Fatalf("link path: id=%s created=%v err=%v", linked.ID, created, err)
	}
	// And the link now resolves that subject to the same account.
	if got, err := s.AccountByGoogleSub("sub-2"); err != nil || got.ID != pw.ID {
		t.Fatalf("by sub after link: id=%s err=%v", got.ID, err)
	}

	// Unknown subject resolves to ErrNotFound.
	if _, err := s.AccountByGoogleSub("nope"); err != ErrNotFound {
		t.Fatalf("unknown sub want ErrNotFound, got %v", err)
	}
}
