package inventory

import (
	"context"
	"net"
	"path/filepath"
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

func startBannerServer(t *testing.T) (host string, port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("RFB 003.008\n"))
			_ = c.Close()
		}
	}()
	return "127.0.0.1", ln.Addr().(*net.TCPAddr).Port, func() { _ = ln.Close() }
}

func TestAddProbeUpdateRemove(t *testing.T) {
	host, port, stop := startBannerServer(t)
	defer stop()

	inv := testInventory(t)
	var changes atomic.Int32
	inv.OnChange(func() { changes.Add(1) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	v, err := inv.Add(ctx, AddRequest{
		Name: "Mock", Host: host, Port: port, Password: "pw", Expose: true, Platform: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Online {
		t.Fatalf("expected online, got %+v", v)
	}
	if v.Banner != "RFB 003.008" {
		t.Fatalf("banner = %q", v.Banner)
	}
	if v.VNCPort < 5901 || v.VNCPort > 5999 {
		t.Fatalf("vnc port = %d", v.VNCPort)
	}
	if v.Token == "" {
		t.Fatal("token is empty")
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
	if changes.Load() < 3 {
		t.Fatalf("onChange calls = %d", changes.Load())
	}
}

func TestProbeOffline(t *testing.T) {
	inv := testInventory(t)
	// Nothing listens on this port.
	v, err := inv.Add(context.Background(), AddRequest{Name: "Off", Host: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatal(err)
	}
	if v.Online {
		t.Fatal("expected offline")
	}
	if v.LastError == "" {
		t.Fatal("expected an error")
	}
}

func TestAddRejectsInvalidMode(t *testing.T) {
	inv := testInventory(t)
	_, err := inv.Add(context.Background(), AddRequest{Name: "Bad", Host: "127.0.0.1", Mode: "nope"})
	if err == nil {
		t.Fatal("expected an error")
	}
}
