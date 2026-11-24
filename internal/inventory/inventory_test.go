package inventory

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"abacad/internal/config"
)

func testInventory(t *testing.T) *Inventory {
	t.Helper()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg)
}

func TestAddUpdateRemove(t *testing.T) {
	inv := testInventory(t)
	var changes atomic.Int32
	inv.OnChange(func() { changes.Add(1) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	v, err := inv.Add(ctx, AddRequest{
		Name: "Mock", Host: "127.0.0.1", Port: 5900, Password: "pw",
		Expose: true, Platform: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Online || v.Known {
		t.Fatalf("new device must stay unseen until a session: %+v", v)
	}
	if v.VNCPort < 5901 || v.VNCPort > 5999 {
		t.Fatalf("vnc port = %d", v.VNCPort)
	}
	if v.Token == "" {
		t.Fatal("token is empty")
	}
	if changes.Load() != 1 {
		t.Fatalf("onChange calls = %d, want 1", changes.Load())
	}

	// Only a real authenticated session makes a device online.
	inv.MarkOnline(v.ID)
	v, _ = inv.Device(v.ID)
	if !v.Online || !v.Known || v.LastSeen == nil {
		t.Fatalf("after session: %+v", v)
	}

	newName := "Renamed"
	v, err = inv.Update(ctx, v.ID, UpdateRequest{Name: &newName})
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "Renamed" || !v.Online {
		t.Fatalf("rename should preserve status: %+v", v)
	}

	// Changing the address invalidates the old observation.
	newHost := "127.0.0.2"
	v, err = inv.Update(ctx, v.ID, UpdateRequest{Host: &newHost})
	if err != nil {
		t.Fatal(err)
	}
	if v.Host != newHost || v.Online || v.Known {
		t.Fatalf("new endpoint must reset status: %+v", v)
	}

	expose := false
	v, err = inv.Update(ctx, v.ID, UpdateRequest{Expose: &expose})
	if err != nil {
		t.Fatal(err)
	}
	if v.VNCPort != 0 {
		t.Fatalf("vnc port after release = %d", v.VNCPort)
	}

	if err := inv.Remove(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.Device(v.ID); ok {
		t.Fatal("device survived remove")
	}
	if changes.Load() != 5 {
		t.Fatalf("onChange calls = %d, want 5", changes.Load())
	}
}

func TestAddNeverConnectsToTheDevice(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- struct{}{}
			_ = c.Close()
		}
	}()

	inv := testInventory(t)
	ctx := context.Background()
	addr := ln.Addr().(*net.TCPAddr)
	v, err := inv.Add(ctx, AddRequest{Name: "Quiet", Host: "127.0.0.1", Port: addr.Port, Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	name := "Still quiet"
	if _, err := inv.Update(ctx, v.ID, UpdateRequest{Name: &name}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-accepted:
		t.Fatal("abacad opened a connection while adding or editing")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAddRejectsInvalidMode(t *testing.T) {
	inv := testInventory(t)
	_, err := inv.Add(context.Background(), AddRequest{Name: "Bad", Host: "127.0.0.1", Mode: "nope"})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestAddARDAccount(t *testing.T) {
	inv := testInventory(t)

	v, err := inv.Add(context.Background(), AddRequest{
		Name: "Mac mini", Host: "192.168.1.20", Auth: config.AuthARD,
		Username: "li", Password: "account-pw", Platform: "macos",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Auth != config.AuthARD || v.Username != "li" {
		t.Fatalf("device = %+v", v)
	}
	reloaded, err := config.Load(inv.Config().Path())
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reloaded.Device(v.ID)
	if !ok || d.Auth != config.AuthARD || d.Username != "li" {
		t.Fatalf("reloaded = %+v, ok=%v", d, ok)
	}
}

func TestStatusStales(t *testing.T) {
	inv := testInventory(t)
	v, err := inv.Add(context.Background(), AddRequest{Name: "Stale", Host: "127.0.0.1", Port: 5900})
	if err != nil {
		t.Fatal(err)
	}
	inv.MarkOnline(v.ID)

	inv.mu.Lock()
	st := inv.status[v.ID]
	st.LastSeen = time.Now().UTC().Add(-staleAfter - time.Minute)
	inv.status[v.ID] = st
	inv.mu.Unlock()

	v, _ = inv.Device(v.ID)
	if v.Online {
		t.Fatal("stale session should not stay online")
	}
	if !v.Known {
		t.Fatal("a device that has been seen should stay known")
	}
}

func TestMarkOfflineAndSessionError(t *testing.T) {
	inv := testInventory(t)
	v, err := inv.Add(context.Background(), AddRequest{Name: "Off", Host: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv.MarkOffline(v.ID, errors.New("connection refused"))
	v, _ = inv.Device(v.ID)
	if v.Online || !v.Known {
		t.Fatalf("after a failed session: %+v", v)
	}
	if !strings.Contains(v.LastError, "connection refused") {
		t.Fatalf("last error = %q", v.LastError)
	}

	inv.MarkOnline(v.ID)
	inv.SetSessionError(v.ID, errors.New("viewer closed"))
	v, _ = inv.Device(v.ID)
	if !v.Online {
		t.Fatal("a failure after authentication should keep the online state")
	}
	if !strings.Contains(v.LastError, "viewer closed") {
		t.Fatalf("last error = %q", v.LastError)
	}
}

func TestDirectModeHasNoAgentPort(t *testing.T) {
	inv := testInventory(t)
	v, err := inv.Add(context.Background(), AddRequest{
		Name: "Catalog", Host: "192.168.1.30", Port: 5900,
		Mode: config.ModeDirect, Expose: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.VNCPort != 0 {
		t.Fatalf("direct mode allocated port %d", v.VNCPort)
	}
}

func TestExposeAllocatesUniquePorts(t *testing.T) {
	inv := testInventory(t)
	ctx := context.Background()
	first, err := inv.Add(ctx, AddRequest{Name: "One", Host: "192.168.1.31", Expose: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := inv.Add(ctx, AddRequest{Name: "Two", Host: "192.168.1.32", Expose: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.VNCPort == 0 || second.VNCPort == 0 || first.VNCPort == second.VNCPort {
		t.Fatalf("ports = %d, %d", first.VNCPort, second.VNCPort)
	}
}
