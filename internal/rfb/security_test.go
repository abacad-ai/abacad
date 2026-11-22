package rfb

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// blacklistedServer returns RFB 3.3's "security type 0" followed by TigerVNC's
// blacklist reason.
func blacklistedServer(conn net.Conn) error {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(conn, "RFB 003.003\n"); err != nil {
		return err
	}
	br := bufio.NewReader(conn)
	if _, err := br.ReadString('\n'); err != nil {
		return err
	}
	if err := binary.Write(conn, binary.BigEndian, uint32(0)); err != nil {
		return err
	}
	reason := "Too many security failures"
	if err := binary.Write(conn, binary.BigEndian, uint32(len(reason))); err != nil {
		return err
	}
	_, err := io.WriteString(conn, reason)
	return err
}

func TestProbeDetectsBlacklist(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = blacklistedServer(conn)
		}
	}()

	_, err = Probe(context.Background(), ProbeOptions{Addr: ln.Addr().String(), Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsTooManySecurityFailures(err) {
		t.Fatalf("error %q not recognized as a blacklist", err)
	}
	if !IsAuthFailure(err) {
		t.Fatalf("blacklist should count as an authentication failure")
	}
	if !strings.Contains(err.Error(), "Too many security failures") {
		t.Fatalf("reason lost: %q", err)
	}
}

func TestProxyForwardsBlacklistReason(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = blacklistedServer(conn)
		}
	}()

	downClient, downProxy := net.Pipe()
	defer downClient.Close()
	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- Proxy(context.Background(), downProxy, Options{
			UpstreamAddr:   ln.Addr().String(),
			DownstreamAuth: AuthNone,
			Timeout:        2 * time.Second,
		})
	}()

	_ = downClient.SetDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(downClient)
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(downClient, "RFB 003.008\n"); err != nil {
		t.Fatal(err)
	}
	n, err := br.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	types := make([]byte, int(n))
	if _, err := io.ReadFull(br, types); err != nil {
		t.Fatal(err)
	}
	if len(types) != 1 || types[0] != 1 {
		t.Fatalf("downstream types = %v", types)
	}
	if _, err := downClient.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	var result uint32
	if err := binary.Read(br, binary.BigEndian, &result); err != nil {
		t.Fatal(err)
	}
	if result != 1 {
		t.Fatalf("security result = %d, want failure", result)
	}
	var reasonLen uint32
	if err := binary.Read(br, binary.BigEndian, &reasonLen); err != nil {
		t.Fatal(err)
	}
	reason := make([]byte, reasonLen)
	if _, err := io.ReadFull(br, reason); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reason), "Too many security failures") {
		t.Fatalf("downstream reason = %q", reason)
	}
	select {
	case <-proxyErr:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not return")
	}
}

func TestProxyDoesNotConnectBeforeClientVersion(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- struct{}{}
			_ = conn.Close()
		}
	}()

	downClient, downProxy := net.Pipe()
	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- Proxy(context.Background(), downProxy, Options{
			UpstreamAddr:   ln.Addr().String(),
			DownstreamAuth: AuthVNC,
			Token:          "Abcd1234",
			Timeout:        time.Second,
		})
	}()

	// The client vanishes without reading the banner or sending a version.
	_ = downClient.Close()
	select {
	case <-accepted:
		t.Fatal("proxy touched the device before the client negotiated")
	case <-time.After(300 * time.Millisecond):
	}
	select {
	case err := <-proxyErr:
		if err == nil {
			t.Fatal("expected the proxy to stop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not return")
	}
}

func TestProxyRejectsBadTokenWithoutTouch(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- struct{}{}
			_ = conn.Close()
		}
	}()

	downClient, downProxy := net.Pipe()
	defer downClient.Close()
	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- Proxy(context.Background(), downProxy, Options{
			UpstreamAddr:   ln.Addr().String(),
			DownstreamAuth: AuthVNC,
			Token:          "Abcd1234",
			Timeout:        time.Second,
		})
	}()

	_ = downClient.SetDeadline(time.Now().Add(2 * time.Second))
	br := bufio.NewReader(downClient)
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(downClient, "RFB 003.008\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := br.ReadByte(); err != nil { // type count
		t.Fatal(err)
	}
	if _, err := br.ReadByte(); err != nil { // type
		t.Fatal(err)
	}
	if _, err := downClient.Write([]byte{2}); err != nil {
		t.Fatal(err)
	}
	challenge := make([]byte, 16)
	if _, err := io.ReadFull(br, challenge); err != nil {
		t.Fatal(err)
	}
	if _, err := downClient.Write(Response(challenge, "Wrong999")); err != nil {
		t.Fatal(err)
	}
	var result uint32
	if err := binary.Read(br, binary.BigEndian, &result); err != nil {
		t.Fatal(err)
	}
	if result != 1 {
		t.Fatalf("security result = %d", result)
	}

	select {
	case err := <-proxyErr:
		if !IsDownstreamAuthFailure(err) {
			t.Fatalf("error = %v, want downstream auth failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not return")
	}
	select {
	case <-accepted:
		t.Fatal("a bad hub token must not reach the device")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSecurityErrorMessageIsReadable(t *testing.T) {
	err := &SecurityError{Reason: "Too many security failures"}
	if !strings.Contains(fmt.Sprint(err), "Too many") {
		t.Fatal(err)
	}
}

func TestBlacklistHintIsASCII(t *testing.T) {
	for _, r := range TooManySecurityFailuresHint {
		if r > 127 {
			t.Fatalf("RFB reason must stay ASCII for client compatibility: %q", TooManySecurityFailuresHint)
		}
	}
	if !strings.Contains(TooManySecurityFailuresHint, "Too many security failures") {
		t.Fatalf("hint lost the server phrase: %q", TooManySecurityFailuresHint)
	}
}
