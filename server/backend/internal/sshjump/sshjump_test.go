package sshjump

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestDeviceIDFromHost(t *testing.T) {
	cases := []struct {
		host, base, want string
		wantErr          bool
	}{
		{"ab3xk9t2wq.abacad.ai", "abacad.ai", "ab3xk9t2wq", false}, // bare base32 id (current format)
		{"dev-ab3x.abacad.ai", "abacad.ai", "dev_ab3x", false},    // legacy prefixed id still round-trips
		{"DEV-AB3X.Abacad.AI", "abacad.ai", "dev_ab3x", false},    // case-insensitive
		{"dev-ab3x.abacad.ai.", "abacad.ai", "dev_ab3x", false},   // trailing dot
		{"dev-ab3x.example.com", "abacad.ai", "", true},         // wrong domain
		{"abacad.ai", "abacad.ai", "", true},                    // no label
		{"a.b.abacad.ai", "abacad.ai", "", true},                // multi-label
	}
	for _, c := range cases {
		got, err := DeviceIDFromHost(c.host, c.base)
		if c.wantErr {
			if err == nil {
				t.Errorf("DeviceIDFromHost(%q) = %q, want error", c.host, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("DeviceIDFromHost(%q) unexpected error: %v", c.host, err)
		}
		if got != c.want {
			t.Errorf("DeviceIDFromHost(%q) = %q, want %q", c.host, got, c.want)
		}
	}
	// Round-trips with HostForDevice — a bare base32 id is a valid DNS label as-is.
	if h := HostForDevice("ab3xk9t2wq", "abacad.ai"); h != "ab3xk9t2wq.abacad.ai" {
		t.Errorf("HostForDevice(bare) = %q", h)
	}
	// Legacy prefixed ids still round-trip via the '_'<->'-' swap.
	if h := HostForDevice("dev_ab3x", "abacad.ai"); h != "dev-ab3x.abacad.ai" {
		t.Errorf("HostForDevice(legacy) = %q", h)
	}
}

// newSigner returns a fresh ed25519 ssh signer for tests.
func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sg, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatal(err)
	}
	return sg
}

// startJump wires a Server whose OpenTunnel echoes bytes back and records the
// device id it was asked for. Returns the listen addr and a pointer to the last
// requested device id.
func startJump(t *testing.T, authorized ssh.PublicKey, accountForKey func(ssh.PublicKey) (string, error), openErr error) (addr string, lastDevice *string) {
	t.Helper()
	host := newSigner(t)
	lastDevice = new(string)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	srv := &Server{
		BaseDomain:    "abacad.ai",
		HostSigner:    host,
		AccountForKey: accountForKey,
		OpenTunnel: func(_ context.Context, accountID, deviceID string) (io.ReadWriteCloser, error) {
			*lastDevice = deviceID
			if openErr != nil {
				return nil, openErr
			}
			// An in-memory echo endpoint standing in for the device's sshd.
			c, s := net.Pipe()
			go io.Copy(s, s) //nolint:errcheck // echo until closed
			return c, nil
		},
	}
	go srv.Serve(ln) //nolint:errcheck

	return ln.Addr().String(), lastDevice
}

func dialJump(t *testing.T, addr string, clientKey ssh.Signer) (*ssh.Client, error) {
	t.Helper()
	return ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            "anyone", // jump ignores the username; auth is by key
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientKey)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	})
}

func TestJumpRoutesToOwnedDevice(t *testing.T) {
	clientKey := newSigner(t)
	wantFP := ssh.FingerprintSHA256(clientKey.PublicKey())
	addr, lastDevice := startJump(t, clientKey.PublicKey(),
		func(k ssh.PublicKey) (string, error) {
			if ssh.FingerprintSHA256(k) == wantFP {
				return "acc_1", nil
			}
			return "", errors.New("unknown key")
		}, nil)

	client, err := dialJump(t, addr, clientKey)
	if err != nil {
		t.Fatalf("dial jump: %v", err)
	}
	defer client.Close()

	// Open a direct-tcpip channel to the device — this is exactly what ProxyJump
	// (ssh -W dev-ab3x.abacad.ai:22) does under the hood.
	conn, err := client.Dial("tcp", "dev-ab3x.abacad.ai:22")
	if err != nil {
		t.Fatalf("channel dial: %v", err)
	}
	defer conn.Close()

	msg := []byte("hello device\n")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != string(msg) {
		t.Errorf("echo = %q, want %q", buf, msg)
	}
	if *lastDevice != "dev_ab3x" {
		t.Errorf("routed to device %q, want dev_ab3x", *lastDevice)
	}
}

func TestJumpRejectsUnknownKey(t *testing.T) {
	addr, _ := startJump(t, nil,
		func(ssh.PublicKey) (string, error) { return "", errors.New("unknown key") }, nil)

	// A client whose key isn't registered can't even complete the handshake.
	if _, err := dialJump(t, addr, newSigner(t)); err == nil {
		t.Fatal("expected auth failure for unregistered key")
	}
}

func TestJumpRejectsUnroutableDevice(t *testing.T) {
	clientKey := newSigner(t)
	addr, _ := startJump(t, clientKey.PublicKey(),
		func(ssh.PublicKey) (string, error) { return "acc_1", nil },
		errors.New("device offline")) // OpenTunnel always fails

	client, err := dialJump(t, addr, clientKey)
	if err != nil {
		t.Fatalf("dial jump: %v", err)
	}
	defer client.Close()

	if conn, err := client.Dial("tcp", "dev-ab3x.abacad.ai:22"); err == nil {
		conn.Close()
		t.Fatal("expected channel rejection when device is unroutable")
	}
}
