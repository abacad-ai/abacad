//go:build vnce2e

// Package vnc real end-to-end test: a REAL VNC server (TigerVNC's Xvnc) on the
// device side, the REAL server bridge in the middle, and a REAL (minimal) RFB
// client on the viewer side — proving that genuine VNC traffic (real handshake,
// real mature server) traverses the ingress→bridge→watch path and a client decodes
// a real framebuffer. This is the correct-architecture proof the hand-rolled Raw
// servers never were. The bridge is byte-transparent, so validating one real server
// through it validates any (the Linux client shells to x11vnc, TigerVNC's sibling).
// Needs Xvnc (apt: tigervnc-standalone-server):
//
//	go test -tags vnce2e ./internal/vnc/
package vnc

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"abacad/internal/store"
)

const display = ":95"

func TestRealVNCThroughBridge(t *testing.T) {
	// --- REAL VNC server (TigerVNC Xvnc): an X server that natively serves RFB on
	// a loopback port, no auth. This is the device-side "real VNC server". ---
	xvnc, err := exec.LookPath("Xvnc")
	if err != nil {
		t.Skip("Xvnc (tigervnc-standalone-server) not installed")
	}
	vncPort := freePort(t)
	vnc := exec.Command(xvnc, display,
		"-geometry", "1280x800", "-depth", "24",
		"-rfbport", strconv.Itoa(vncPort),
		"-SecurityTypes", "None", "-localhost", "-AlwaysShared",
	)
	if err := vnc.Start(); err != nil {
		t.Fatalf("Xvnc start: %v", err)
	}
	defer func() { _ = vnc.Process.Kill(); _, _ = vnc.Process.Wait() }()
	vncAddr := "127.0.0.1:" + strconv.Itoa(vncPort)
	if err := waitListen(vncAddr, 5*time.Second); err != nil {
		t.Fatalf("Xvnc did not listen: %v", err)
	}
	time.Sleep(500 * time.Millisecond) // let it finish RFB init

	// --- the real bridge, with a session registered directly ---
	m := NewManager(nil, "wss://test", func(r *http.Request) (store.Account, error) {
		return store.Account{ID: "acc1"}, nil
	})
	s := &session{
		mgr: m, deviceID: "dev1", accountID: "acc1",
		ingressTok: "itok", ticket: "tkt",
		expiresAt: time.Now().Add(time.Minute),
		done:      make(chan struct{}),
	}
	m.byDevice["dev1"] = s
	m.byIngress["itok"] = s
	m.byTicket["tkt"] = s

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vnc/ingress":
			m.ServeIngress(w, r)
		case "/vnc/watch":
			m.ServeWatch(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http")

	// --- device side: dial the ingress and pipe it to the local x11vnc (this is
	// what the Linux client's vnc verb does) ---
	dev, _, err := websocket.Dial(ctx, wsBase+"/vnc/ingress?token=itok", nil)
	if err != nil {
		t.Fatalf("device ingress dial: %v", err)
	}
	dev.SetReadLimit(1 << 24)
	tcp, err := net.Dial("tcp", vncAddr)
	if err != nil {
		t.Fatalf("dial x11vnc: %v", err)
	}
	go pipeDevice(ctx, dev, tcp)
	defer tcp.Close()

	// --- viewer side: a real minimal RFB client over /vnc/watch ---
	view, _, err := websocket.Dial(ctx, wsBase+"/vnc/watch?ticket=tkt", nil)
	if err != nil {
		t.Fatalf("viewer watch dial: %v", err)
	}
	view.SetReadLimit(1 << 24)
	defer view.Close(websocket.StatusNormalClosure, "")

	w, h := rfbHandshake(t, ctx, view)
	if w != 1280 || h != 800 {
		t.Fatalf("ServerInit dims = %dx%d, want 1280x800", w, h)
	}
	t.Logf("PASS ServerInit from real VNC server through the bridge: %dx%d", w, h)

	// Request the full framebuffer with Raw encoding and decode one real update.
	rw, rh, npix := rfbReadOneUpdate(t, ctx, view, w, h)
	if rw <= 0 || rh <= 0 || npix == 0 {
		t.Fatalf("bad framebuffer update: %dx%d, %d pixel bytes", rw, rh, npix)
	}
	t.Logf("PASS real FramebufferUpdate traversed the bridge: rect %dx%d, %d pixel bytes", rw, rh, npix)
}

// pipeDevice relays bytes between the ingress WebSocket and the local x11vnc TCP
// socket both ways — the Linux client's WS<->TCP pipe, inline.
func pipeDevice(ctx context.Context, ws *websocket.Conn, tcp net.Conn) {
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				if ws.Write(ctx, websocket.MessageBinary, buf[:n]) != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if _, err := tcp.Write(data); err != nil {
			return
		}
	}
}

// rfbReader reads an exact number of bytes from the viewer WebSocket, buffering
// across frames — RFB is a byte stream.
type rfbReader struct {
	ws  *websocket.Conn
	ctx context.Context
	buf []byte
}

func (r *rfbReader) read(n int) ([]byte, error) {
	for len(r.buf) < n {
		_, data, err := r.ws.Read(r.ctx)
		if err != nil {
			return nil, err
		}
		r.buf = append(r.buf, data...)
	}
	out := r.buf[:n]
	r.buf = r.buf[n:]
	return out, nil
}

// rfbHandshake performs the RFB 3.8 client handshake against the bridged x11vnc
// and returns the framebuffer dimensions from ServerInit.
func rfbHandshake(t *testing.T, ctx context.Context, ws *websocket.Conn) (int, int) {
	t.Helper()
	r := &rfbReader{ws: ws, ctx: ctx}
	must := func(n int) []byte {
		b, err := r.read(n)
		if err != nil {
			t.Fatalf("rfb read %d: %v", n, err)
		}
		return b
	}
	write := func(b []byte) {
		if err := ws.Write(ctx, websocket.MessageBinary, b); err != nil {
			t.Fatalf("rfb write: %v", err)
		}
	}

	ver := must(12) // "RFB 003.008\n"
	if !strings.HasPrefix(string(ver), "RFB 003.") {
		t.Fatalf("unexpected ProtocolVersion %q", ver)
	}
	write([]byte("RFB 003.008\n"))
	nTypes := int(must(1)[0]) // number of security types (3.7+)
	if nTypes == 0 {
		t.Fatalf("server refused: %s", must(4))
	}
	types := must(nTypes)
	if !contains(types, 1) {
		t.Fatalf("server does not offer None auth: %v", types)
	}
	write([]byte{1}) // choose None
	res := must(4)   // SecurityResult
	if binary.BigEndian.Uint32(res) != 0 {
		t.Fatalf("security failed: %v", res)
	}
	write([]byte{1}) // ClientInit: shared

	si := must(24) // ServerInit fixed part: 2 w, 2 h, 16 pf, 4 namelen
	w := int(binary.BigEndian.Uint16(si[0:2]))
	h := int(binary.BigEndian.Uint16(si[2:4]))
	nameLen := int(binary.BigEndian.Uint32(si[20:24]))
	if nameLen > 0 {
		must(nameLen)
	}
	return w, h
}

// rfbReadOneUpdate forces Raw encoding, requests the whole framebuffer, and reads
// exactly one FramebufferUpdate — returning the first rectangle's size and pixel
// byte count. (Raw keeps the client trivial; the bridge is byte-transparent, so
// any encoding x11vnc uses would traverse identically.)
func rfbReadOneUpdate(t *testing.T, ctx context.Context, ws *websocket.Conn, w, h int) (int, int, int) {
	t.Helper()
	r := &rfbReader{ws: ws, ctx: ctx}
	must := func(n int) []byte {
		b, err := r.read(n)
		if err != nil {
			t.Fatalf("rfb read %d: %v", n, err)
		}
		return b
	}
	write := func(b []byte) {
		if err := ws.Write(ctx, websocket.MessageBinary, b); err != nil {
			t.Fatalf("rfb write: %v", err)
		}
	}

	// SetEncodings: message 2, pad, count=1, encoding 0 (Raw).
	setEnc := []byte{2, 0, 0, 1, 0, 0, 0, 0}
	write(setEnc)
	// FramebufferUpdateRequest: message 3, incremental=0, x=0,y=0,w,h.
	req := make([]byte, 10)
	req[0] = 3
	binary.BigEndian.PutUint16(req[6:8], uint16(w))
	binary.BigEndian.PutUint16(req[8:10], uint16(h))
	write(req)

	// Wait for a FramebufferUpdate (skip any other server messages).
	for {
		msgType := must(1)[0]
		switch msgType {
		case 0: // FramebufferUpdate
			hdr := must(3) // pad + num-rects
			nRects := int(binary.BigEndian.Uint16(hdr[1:3]))
			if nRects == 0 {
				// Empty update (no change yet); ask again.
				write(req)
				continue
			}
			rect := must(12) // x,y,w,h,encoding
			rw := int(binary.BigEndian.Uint16(rect[4:6]))
			rh := int(binary.BigEndian.Uint16(rect[6:8]))
			enc := int32(binary.BigEndian.Uint32(rect[8:12]))
			if enc != 0 {
				t.Fatalf("expected Raw encoding, got %d", enc)
			}
			pixels := rw * rh * 4 // 32bpp
			must(pixels)
			return rw, rh, pixels
		case 1: // SetColourMapEntries
			skip := must(5)
			nColors := int(binary.BigEndian.Uint16(skip[3:5]))
			must(nColors * 6)
		case 2: // Bell
		case 3: // ServerCutText
			skip := must(7)
			n := int(binary.BigEndian.Uint32(skip[3:7]))
			must(n)
		default:
			t.Fatalf("unexpected server message %d", msgType)
		}
	}
}

func contains(b []byte, v byte) bool {
	for _, x := range b {
		if x == v {
			return true
		}
	}
	return false
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitListen(addr string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}
