package agent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// startMockRFB stands up a localhost TCP server that behaves like a VNC server:
// it sends the RFB banner on connect, then echoes. It stands in for x11vnc.
func startMockRFB(t *testing.T) (addr string, stop func()) {
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
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte("RFB 003.008\n"))
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						_, _ = c.Write(buf[:n]) // echo
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

// TestVNCPipe drives the Linux vnc handler with a mock RFB server (in place of
// x11vnc) and a mock ingress WebSocket, and verifies RFB bytes flow both ways:
// device -> ingress (the banner) and ingress -> device -> RFB -> back (the echo).
func TestVNCPipe(t *testing.T) {
	rfbAddr, stopRFB := startMockRFB(t)
	defer stopRFB()

	orig := startLocalVNC
	startLocalVNC = func(display string) (string, func(), error) { return rfbAddr, func() {}, nil }
	defer func() { startLocalVNC = orig }()

	fromDevice := make(chan []byte, 8)
	toDevice := make(chan []byte, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		ctx := r.Context()
		go func() {
			for {
				_, data, err := c.Read(ctx)
				if err != nil {
					return
				}
				fromDevice <- data
			}
		}()
		for msg := range toDevice {
			if err := c.Write(ctx, websocket.MessageBinary, msg); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/vnc/ingress?token=x"

	v := newVNCHandler()
	res, err := v.handle(map[string]any{"action": "start", "url": wsURL})
	if err != nil {
		t.Fatalf("vnc start: %v", err)
	}
	if res["started"] != true {
		t.Fatalf("start result %v", res)
	}
	defer v.stop()

	// The device should relay the mock RFB banner to the ingress.
	select {
	case got := <-fromDevice:
		if string(got) != "RFB 003.008\n" {
			t.Fatalf("banner = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no RFB banner relayed from device")
	}

	// Input flows the other way: ingress -> device -> RFB (echo) -> device -> ingress.
	toDevice <- []byte("ping")
	select {
	case got := <-fromDevice:
		if string(got) != "ping" {
			t.Fatalf("echo = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no echo back through the pipe")
	}
}
