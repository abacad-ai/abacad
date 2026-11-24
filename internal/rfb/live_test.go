package rfb

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// TestLiveServer is an opt-in end-to-end check against a real VNC server (or a
// hub port in front of one). It is skipped unless ABACAD_LIVE_VNC is set:
//
//	ABACAD_LIVE_VNC=127.0.0.1:5902 ABACAD_LIVE_TOKEN=Abcd1234 go test ./internal/rfb -run TestLiveServer -v
func TestLiveServer(t *testing.T) {
	addr := os.Getenv("ABACAD_LIVE_VNC")
	token := os.Getenv("ABACAD_LIVE_TOKEN")
	if addr == "" || token == "" {
		t.Skip("set ABACAD_LIVE_VNC and ABACAD_LIVE_TOKEN to run")
	}
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(conn)

	banner, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("banner: %q", banner)
	if _, err := io.WriteString(conn, "RFB 003.008\n"); err != nil {
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
	t.Logf("security types: %v", types)
	if err := writeByte(conn, 2); err != nil {
		t.Fatal(err)
	}
	challenge := make([]byte, 16)
	if _, err := io.ReadFull(br, challenge); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(Response(challenge, token)); err != nil {
		t.Fatal(err)
	}
	var result uint32
	if err := binary.Read(br, binary.BigEndian, &result); err != nil {
		t.Fatal(err)
	}
	if result != 0 {
		t.Fatalf("auth failed: %d", result)
	}
	if err := writeByte(conn, 1); err != nil {
		t.Fatal(err)
	}
	hdr := make([]byte, 24)
	if _, err := io.ReadFull(br, hdr); err != nil {
		t.Fatal(err)
	}
	w := binary.BigEndian.Uint16(hdr[0:2])
	h := binary.BigEndian.Uint16(hdr[2:4])
	nameLen := binary.BigEndian.Uint32(hdr[20:24])
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(br, name); err != nil {
		t.Fatal(err)
	}
	t.Logf("desktop: %dx%d %q", w, h, name)

	// Ask for 32bpp true colour so Raw rectangles are easy to size.
	pf := []byte{32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0}
	msg := append([]byte{0, 0, 0, 0}, pf...)
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	// SetEncodings: Raw only.
	if _, err := conn.Write([]byte{2, 0, 0, 1, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	req := make([]byte, 10)
	req[0] = 3
	binary.BigEndian.PutUint16(req[6:8], w)
	binary.BigEndian.PutUint16(req[8:10], h)
	if _, err := conn.Write(req); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(br, head); err != nil {
		t.Fatal(err)
	}
	if head[0] != 0 {
		t.Fatalf("expected FramebufferUpdate, got message type %d", head[0])
	}
	rects := binary.BigEndian.Uint16(head[2:4])
	t.Logf("framebuffer update: %d rect(s)", rects)
	for i := 0; i < int(rects); i++ {
		rh := make([]byte, 12)
		if _, err := io.ReadFull(br, rh); err != nil {
			t.Fatal(err)
		}
		rw := binary.BigEndian.Uint16(rh[4:6])
		rhgt := binary.BigEndian.Uint16(rh[6:8])
		enc := int32(binary.BigEndian.Uint32(rh[8:12]))
		if enc != 0 {
			t.Fatalf("unexpected encoding %d (only Raw requested)", enc)
		}
		pixels := make([]byte, int(rw)*int(rhgt)*4)
		if _, err := io.ReadFull(br, pixels); err != nil {
			t.Fatal(err)
		}
		t.Logf("rect %d: %dx%d raw %d bytes", i, rw, rhgt, len(pixels))
	}
	fmt.Println("live VNC session: ok")
}
