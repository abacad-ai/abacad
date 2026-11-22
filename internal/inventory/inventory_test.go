package inventory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"abacad/internal/config"
	"abacad/internal/rfb"
)

func testInventory(t *testing.T) *Inventory {
	t.Helper()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg)
}

// startMockVNC runs a minimal RFB 3.8 server that accepts exactly one VNC
// password. Every accepted connection is counted so tests can prove that a
// health check authenticated instead of just opening TCP.
func startMockVNC(t *testing.T, password string) (host string, port int, conns *atomic.Int32, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conns = &atomic.Int32{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go serveMockVNC(c, password)
		}
	}()
	return "127.0.0.1", ln.Addr().(*net.TCPAddr).Port, conns, func() { _ = ln.Close() }
}

func serveMockVNC(conn net.Conn, password string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "RFB 003.008\n"); err != nil {
		return
	}
	br := bufio.NewReader(conn)
	if _, err := br.ReadString('\n'); err != nil {
		return
	}
	if _, err := conn.Write([]byte{1, 2}); err != nil {
		return
	}
	selected, err := br.ReadByte()
	if err != nil || selected != 2 {
		return
	}
	challenge := []byte("0123456789abcdef")
	if _, err := conn.Write(challenge); err != nil {
		return
	}
	resp := make([]byte, 16)
	if _, err := io.ReadFull(br, resp); err != nil {
		return
	}
	if !bytes.Equal(resp, rfb.Response(challenge, password)) {
		_ = binary.Write(conn, binary.BigEndian, uint32(1))
		_ = binary.Write(conn, binary.BigEndian, uint32(3))
		_, _ = io.WriteString(conn, "bad")
		return
	}
	if err := binary.Write(conn, binary.BigEndian, uint32(0)); err != nil {
		return
	}
	if _, err := br.ReadByte(); err != nil { // ClientInit
		return
	}
	hdr := make([]byte, 24)
	binary.BigEndian.PutUint16(hdr[0:2], 800)
	binary.BigEndian.PutUint16(hdr[2:4], 600)
	hdr[4] = 32
	hdr[5] = 24
	hdr[7] = 1
	binary.BigEndian.PutUint16(hdr[8:10], 255)
	binary.BigEndian.PutUint16(hdr[10:12], 255)
	binary.BigEndian.PutUint16(hdr[12:14], 255)
	hdr[14], hdr[15] = 16, 8
	name := "mock-desktop"
	binary.BigEndian.PutUint32(hdr[20:24], uint32(len(name)))
	if _, err := conn.Write(hdr); err != nil {
		return
	}
	if _, err := io.WriteString(conn, name); err != nil {
		return
	}
	// A real client would keep the session open; the probe closes here.
	_, _ = io.Copy(io.Discard, br)
}

func TestAddProbeUpdateRemove(t *testing.T) {
	host, port, conns, stop := startMockVNC(t, "pw")
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
	if !v.Online || !v.Known {
		t.Fatalf("expected online and known, got %+v", v)
	}
	if v.Banner != "RFB 003.008" {
		t.Fatalf("banner = %q", v.Banner)
	}
	if v.Detail != "mock-desktop (800x600)" {
		t.Fatalf("detail = %q", v.Detail)
	}
	if v.VNCPort < 5901 || v.VNCPort > 5999 {
		t.Fatalf("vnc port = %d", v.VNCPort)
	}
	if v.Token == "" {
		t.Fatal("token is empty")
	}
	if conns.Load() != 1 {
		t.Fatalf("connections = %d, want 1 authenticated probe", conns.Load())
	}

	// A second manual VNC check is throttled and must not touch the server.
	before := conns.Load()
	st := inv.Probe(ctx, v.ID)
	if conns.Load() != before {
		t.Fatalf("throttled check opened a connection (%d -> %d)", before, conns.Load())
	}
	if !st.Throttled || !strings.Contains(st.ThrottleMessage, "limited to one every") {
		t.Fatalf("throttle = %v %q", st.Throttled, st.ThrottleMessage)
	}

	newName := "Renamed"
	newHost := host
	newPort := port
	v, err = inv.Update(ctx, v.ID, UpdateRequest{Name: &newName, Host: &newHost, Port: &newPort})
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "Renamed" || v.Host != host || v.Port != port {
		t.Fatalf("update did not apply: %+v", v)
	}
	if !v.Online {
		t.Fatalf("expected online after re-check, got %+v", v)
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
	if !v.Known {
		t.Fatal("expected the failed check to be known")
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

func TestAddARDAccount(t *testing.T) {
	host, port, _, stop := startMockVNC(t, "account-pw")
	defer stop()

	inv := testInventory(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	v, err := inv.Add(ctx, AddRequest{
		Name: "Mac mini", Host: host, Port: port, Auth: config.AuthARD,
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

func TestHealthCommand(t *testing.T) {
	inv := testInventory(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	v, err := inv.Add(ctx, AddRequest{
		Name: "Debian", Host: "127.0.0.1", Port: 5902,
		Health: config.HealthCommand, HealthCommand: "printf active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Online || !v.Known || v.Detail != "active" {
		t.Fatalf("device = %+v", v)
	}

	v, err = inv.Update(ctx, v.ID, UpdateRequest{
		HealthCommand: strPtr("printf inactive; exit 3"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Online || !v.Known {
		t.Fatalf("device = %+v", v)
	}
	if !strings.Contains(v.LastError, "exit status 3") {
		t.Fatalf("last error = %q", v.LastError)
	}

	// Fixing the command recovers immediately; a failed check does not wedge
	// the device behind the backoff.
	v, err = inv.Update(ctx, v.ID, UpdateRequest{
		HealthCommand: strPtr("printf active"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Online || v.Detail != "active" {
		t.Fatalf("device did not recover: %+v", v)
	}
}

func TestAuthenticatedProbesThenRealClient(t *testing.T) {
	host, port, conns, stop := startMockVNC(t, "pw")
	defer stop()
	addr := fmt.Sprintf("%s:%d", host, port)

	// Ten health probes, each a complete handshake and authentication. This is
	// the acceptance case: a server that counts unauthenticated disconnects
	// must never see a failure from these.
	for i := 0; i < 10; i++ {
		res, err := rfb.Probe(context.Background(), rfb.ProbeOptions{
			Addr: addr, Password: "pw", Timeout: 3 * time.Second,
		})
		if err != nil {
			t.Fatalf("probe %d: %v", i+1, err)
		}
		if res.DesktopName != "mock-desktop" {
			t.Fatalf("probe %d desktop = %q", i+1, res.DesktopName)
		}
	}
	if conns.Load() != 10 {
		t.Fatalf("connections = %d", conns.Load())
	}

	// A real client still authenticates afterwards.
	downClient, downProxy := net.Pipe()
	defer downClient.Close()
	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- rfb.Proxy(context.Background(), downProxy, rfb.Options{
			UpstreamAddr: addr, UpstreamPass: "pw",
			DownstreamAuth: rfb.AuthVNC, Token: "Abcd1234", Timeout: 3 * time.Second,
		})
	}()
	if err := mockTokenClient(downClient, "Abcd1234"); err != nil {
		t.Fatalf("real client: %v", err)
	}
	_ = downClient.Close()
	select {
	case err := <-proxyErr:
		if err != nil {
			t.Fatalf("proxy: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not finish")
	}
}

func mockTokenClient(conn net.Conn, token string) error {
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)
	if _, err := br.ReadString('\n'); err != nil {
		return err
	}
	if _, err := io.WriteString(conn, "RFB 003.008\n"); err != nil {
		return err
	}
	n, err := br.ReadByte()
	if err != nil {
		return err
	}
	types := make([]byte, int(n))
	if _, err := io.ReadFull(br, types); err != nil {
		return err
	}
	if len(types) != 1 || types[0] != 2 {
		return fmt.Errorf("types = %v", types)
	}
	if _, err := conn.Write([]byte{2}); err != nil {
		return err
	}
	challenge := make([]byte, 16)
	if _, err := io.ReadFull(br, challenge); err != nil {
		return err
	}
	if _, err := conn.Write(rfb.Response(challenge, token)); err != nil {
		return err
	}
	var result uint32
	if err := binary.Read(br, binary.BigEndian, &result); err != nil {
		return err
	}
	if result != 0 {
		return fmt.Errorf("hub rejected token: %d", result)
	}
	if _, err := conn.Write([]byte{1}); err != nil {
		return err
	}
	hdr := make([]byte, 24)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return err
	}
	nameLen := binary.BigEndian.Uint32(hdr[20:24])
	_, err = io.ReadFull(br, make([]byte, nameLen))
	return err
}

func strPtr(s string) *string { return &s }

func TestScheduledVNCHealthUsesOneAuthenticatedProbe(t *testing.T) {
	host, port, conns, stop := startMockVNC(t, "pw")
	defer stop()

	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := cfg.AddDevice(config.Device{
		ID: "scheduled", Name: "Scheduled", Host: host, Port: port,
		Auth: config.AuthVNC, Password: "pw", Mode: config.ModeGateway,
		Health: config.HealthVNC,
	})
	if err != nil {
		t.Fatal(err)
	}
	inv := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go inv.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := inv.Device(d.ID); ok && v.Known {
			if !v.Online {
				t.Fatalf("scheduled health check failed: %+v", v)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if v, _ := inv.Device(d.ID); !v.Known || !v.Online {
		t.Fatalf("device never became online: %+v", v)
	}
	if conns.Load() != 1 {
		t.Fatalf("connections = %d, want one", conns.Load())
	}
	// The scheduler must not probe again inside the five-minute floor.
	time.Sleep(300 * time.Millisecond)
	if conns.Load() != 1 {
		t.Fatalf("connections = %d after waiting", conns.Load())
	}
}

func TestExponentialBackoff(t *testing.T) {
	base := time.Minute
	got := []time.Duration{
		exponentialBackoff(base, 1, time.Hour),
		exponentialBackoff(base, 2, time.Hour),
		exponentialBackoff(base, 3, time.Hour),
	}
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoff[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	if exponentialBackoff(base, 100, 10*time.Minute) != 10*time.Minute {
		t.Fatal("backoff did not cap")
	}
}
