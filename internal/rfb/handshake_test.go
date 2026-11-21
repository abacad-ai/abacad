package rfb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestResponseDeterministic(t *testing.T) {
	challenge := bytes.Repeat([]byte{0x01}, 16)
	a := Response(challenge, "secret")
	b := Response(challenge, "secret")
	if len(a) != 16 || !bytes.Equal(a, b) {
		t.Fatalf("response not deterministic: %x %x", a, b)
	}
	if bytes.Equal(a, Response(challenge, "other")) {
		t.Fatal("different passwords produced the same response")
	}
}

func TestProxyVNCBothSides(t *testing.T) {
	runProxyTest(t, proxyTest{
		upstreamPassword: "device-pass",
		upstreamOffer:    []byte{1, 2},
		downstreamAuth:   AuthVNC,
		token:            "Abcd1234",
	})
}

func TestProxyNoneUpstream(t *testing.T) {
	runProxyTest(t, proxyTest{
		upstreamPassword: "",
		upstreamOffer:    []byte{1},
		downstreamAuth:   AuthVNC,
		token:            "Abcd1234",
	})
}

func TestProxyNoneDownstream(t *testing.T) {
	runProxyTest(t, proxyTest{
		upstreamPassword: "device-pass",
		upstreamOffer:    []byte{1, 2},
		downstreamAuth:   AuthNone,
		token:            "",
	})
}

type proxyTest struct {
	upstreamPassword string
	upstreamOffer    []byte
	downstreamAuth   AuthMode
	token            string
}

func runProxyTest(t *testing.T, tc proxyTest) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		serverErr <- mockServer(conn, tc.upstreamPassword, tc.upstreamOffer)
	}()

	downClient, downProxy := net.Pipe()
	defer downClient.Close()
	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- Proxy(context.Background(), downProxy, Options{
			UpstreamAddr:   ln.Addr().String(),
			UpstreamPass:   tc.upstreamPassword,
			DownstreamAuth: tc.downstreamAuth,
			Token:          tc.token,
			Timeout:        3 * time.Second,
		})
	}()

	if err := mockClient(downClient, tc.downstreamAuth, tc.token); err != nil {
		t.Fatalf("client: %v", err)
	}
	_ = downClient.Close()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not finish")
	}
	select {
	case err := <-proxyErr:
		if err != nil {
			t.Fatalf("proxy: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not finish")
	}
}

func mockServer(conn net.Conn, password string, offer []byte) error {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)

	if _, err := io.WriteString(conn, "RFB 003.008\n"); err != nil {
		return err
	}
	ver, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	if ver != "RFB 003.008\n" {
		return fmt.Errorf("unexpected client version %q", ver)
	}
	if err := writeByte(conn, byte(len(offer))); err != nil {
		return err
	}
	if _, err := conn.Write(offer); err != nil {
		return err
	}
	selected, err := br.ReadByte()
	if err != nil {
		return err
	}
	if !bytes.Contains(offer, []byte{selected}) {
		return fmt.Errorf("client selected an unoffered type %d", selected)
	}
	if selected == 2 {
		challenge := []byte("0123456789abcdef")
		if _, err := conn.Write(challenge); err != nil {
			return err
		}
		resp := make([]byte, 16)
		if _, err := io.ReadFull(br, resp); err != nil {
			return err
		}
		want := Response(challenge, password)
		if !bytes.Equal(resp, want) {
			_ = binary.Write(conn, binary.BigEndian, uint32(1))
			return fmt.Errorf("server rejected password")
		}
	}
	if err := binary.Write(conn, binary.BigEndian, uint32(0)); err != nil {
		return err
	}
	if _, err := br.ReadByte(); err != nil { // ClientInit
		return err
	}
	hdr := make([]byte, 24)
	binary.BigEndian.PutUint16(hdr[0:2], 800)
	binary.BigEndian.PutUint16(hdr[2:4], 600)
	hdr[4] = 32 // bits per pixel
	hdr[5] = 24 // depth
	hdr[6] = 0  // little endian
	hdr[7] = 1  // true colour
	binary.BigEndian.PutUint16(hdr[8:10], 255)
	binary.BigEndian.PutUint16(hdr[10:12], 255)
	binary.BigEndian.PutUint16(hdr[12:14], 255)
	hdr[14] = 16
	hdr[15] = 8
	hdr[16] = 0
	name := "mock"
	binary.BigEndian.PutUint32(hdr[20:24], uint32(len(name)))
	if _, err := conn.Write(hdr); err != nil {
		return err
	}
	if _, err := io.WriteString(conn, name); err != nil {
		return err
	}

	buf := make([]byte, 256)
	for {
		n, err := br.Read(buf)
		if n > 0 {
			if _, werr := conn.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func mockClient(conn net.Conn, mode AuthMode, token string) error {
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)

	banner, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	if banner != "RFB 003.008\n" {
		return fmt.Errorf("unexpected server banner %q", banner)
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
	want := byte(1)
	if mode == AuthVNC {
		want = 2
	}
	if len(types) != 1 || types[0] != want {
		return fmt.Errorf("unexpected security types %v", types)
	}
	if err := writeByte(conn, want); err != nil {
		return err
	}
	if want == 2 {
		challenge := make([]byte, 16)
		if _, err := io.ReadFull(br, challenge); err != nil {
			return err
		}
		if _, err := conn.Write(Response(challenge, token)); err != nil {
			return err
		}
	}
	var result uint32
	if err := binary.Read(br, binary.BigEndian, &result); err != nil {
		return err
	}
	if result != 0 {
		return fmt.Errorf("server rejected auth: %d", result)
	}
	if err := writeByte(conn, 1); err != nil { // ClientInit shared
		return err
	}
	hdr := make([]byte, 24)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return err
	}
	nameLen := binary.BigEndian.Uint32(hdr[20:24])
	if _, err := io.ReadFull(br, make([]byte, nameLen)); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		return err
	}
	echo := make([]byte, 5)
	if _, err := io.ReadFull(br, echo); err != nil {
		return err
	}
	if string(echo) != "hello" {
		return fmt.Errorf("echo = %q", echo)
	}
	return nil
}

func TestProxyRejectsWrongToken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = mockServer(conn, "device-pass", []byte{2})
		}
	}()
	downClient, downProxy := net.Pipe()
	defer downClient.Close()
	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- Proxy(context.Background(), downProxy, Options{
			UpstreamAddr:   ln.Addr().String(),
			UpstreamPass:   "device-pass",
			DownstreamAuth: AuthVNC,
			Token:          "Abcd1234",
			Timeout:        2 * time.Second,
		})
	}()

	// Wrong token: the proxy should report an error instead of piping.
	err = mockClient(downClient, AuthVNC, "Wrong999")
	if err == nil {
		t.Fatal("expected the client to see a rejection")
	}
	select {
	case <-proxyErr:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not return")
	}
}
