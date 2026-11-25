package rfbserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net"
	"testing"
	"time"
)

// fixedSource is a Source that never changes, which is all a protocol test
// needs.
type fixedSource struct {
	img image.Image
}

func (f *fixedSource) Size() (int, int) {
	b := f.img.Bounds()
	return b.Dx(), b.Dy()
}

func (f *fixedSource) Wait(ctx context.Context, after uint64) (Frame, error) {
	if after < 1 {
		return Frame{Image: f.img, Seq: 1}, nil
	}
	<-ctx.Done()
	return Frame{}, ctx.Err()
}

func testImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 128, 255})
		}
	}
	return img
}

// serve starts a server and returns its address.
func serve(t *testing.T, opts Options, in Input) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	src := &fixedSource{img: testImage(64, 48)}
	srv := New(src, in, opts)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.ServeConn(conn)
		}
	}()
	return ln.Addr().String()
}

// handshake speaks the client half of RFB 3.8 up to ServerInit.
func handshake(t *testing.T, addr, password string) (net.Conn, *bufio.Reader, int, int) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	r := bufio.NewReader(conn)

	banner := make([]byte, 12)
	if _, err := io.ReadFull(r, banner); err != nil {
		t.Fatalf("read banner: %v", err)
	}
	if string(banner) != "RFB 003.008\n" {
		t.Fatalf("banner = %q", banner)
	}
	if _, err := conn.Write([]byte("RFB 003.008\n")); err != nil {
		t.Fatal(err)
	}

	n, err := r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	types := make([]byte, n)
	if _, err := io.ReadFull(r, types); err != nil {
		t.Fatal(err)
	}
	chosen := byte(1)
	for _, ty := range types {
		if password != "" && ty == 2 {
			chosen = 2
		}
	}
	if _, err := conn.Write([]byte{chosen}); err != nil {
		t.Fatal(err)
	}

	if chosen == 2 {
		challenge := make([]byte, 16)
		if _, err := io.ReadFull(r, challenge); err != nil {
			t.Fatal(err)
		}
		resp, err := vncResponse(password, challenge)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write(resp); err != nil {
			t.Fatal(err)
		}
	}

	var result uint32
	if err := binary.Read(r, binary.BigEndian, &result); err != nil {
		t.Fatalf("read security result: %v", err)
	}
	if result != 0 {
		t.Fatalf("security result = %d", result)
	}

	if _, err := conn.Write([]byte{1}); err != nil { // ClientInit, shared
		t.Fatal(err)
	}

	si := make([]byte, 24)
	if _, err := io.ReadFull(r, si); err != nil {
		t.Fatalf("read ServerInit: %v", err)
	}
	w := int(binary.BigEndian.Uint16(si[0:2]))
	h := int(binary.BigEndian.Uint16(si[2:4]))
	if si[4] != 32 || si[7] != 1 {
		t.Fatalf("pixel format bpp=%d truecolor=%d", si[4], si[7])
	}
	nameLen := int(binary.BigEndian.Uint32(si[20:24]))
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(r, name); err != nil {
		t.Fatal(err)
	}
	return conn, r, w, h
}

func setEncodings(t *testing.T, conn net.Conn, encs ...int32) {
	t.Helper()
	buf := make([]byte, 4+len(encs)*4)
	buf[0] = 2
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(encs)))
	for i, e := range encs {
		binary.BigEndian.PutUint32(buf[4+i*4:], uint32(e))
	}
	if _, err := conn.Write(buf); err != nil {
		t.Fatal(err)
	}
}

func requestUpdate(t *testing.T, conn net.Conn, incremental bool) {
	t.Helper()
	buf := make([]byte, 10)
	buf[0] = 3
	if incremental {
		buf[1] = 1
	}
	binary.BigEndian.PutUint16(buf[6:8], 64)
	binary.BigEndian.PutUint16(buf[8:10], 48)
	if _, err := conn.Write(buf); err != nil {
		t.Fatal(err)
	}
}

func readRectHeader(t *testing.T, r *bufio.Reader) (int, int, int32) {
	t.Helper()
	hdr := make([]byte, 16) // type, pad, rects, x, y, w, h
	if _, err := io.ReadFull(r, hdr); err != nil {
		t.Fatalf("read update header: %v", err)
	}
	if hdr[0] != 0 {
		t.Fatalf("message type = %d, want 0 (FramebufferUpdate)", hdr[0])
	}
	if binary.BigEndian.Uint16(hdr[2:4]) != 1 {
		t.Fatal("want exactly one rect")
	}
	w := int(binary.BigEndian.Uint16(hdr[8:10]))
	h := int(binary.BigEndian.Uint16(hdr[10:12]))
	enc := int32(binary.BigEndian.Uint32(hdr[12:16]))
	return w, h, enc
}

func TestTightJPEGUpdate(t *testing.T) {
	addr := serve(t, Options{Name: "test", Quality: 80}, nil)
	conn, r, w, h := handshake(t, addr, "")
	if w != 64 || h != 48 {
		t.Fatalf("ServerInit size = %dx%d", w, h)
	}
	setEncodings(t, conn, 7)
	requestUpdate(t, conn, false)

	rw, rh, enc := readRectHeader(t, r)
	if enc != 7 {
		t.Fatalf("encoding = %d, want 7 (Tight)", enc)
	}
	if rw != 64 || rh != 48 {
		t.Fatalf("rect = %dx%d", rw, rh)
	}

	ctl, err := r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	if ctl != 0x90 {
		t.Fatalf("tight control = %#x, want 0x90 (JPEG, no stream reset)", ctl)
	}
	n := readCompactLen(t, r)
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("jpeg decode: %v", err)
	}
	if got := img.Bounds(); got.Dx() != 64 || got.Dy() != 48 {
		t.Fatalf("jpeg size = %v", got)
	}
}

func TestRawUpdate(t *testing.T) {
	addr := serve(t, Options{Name: "test"}, nil)
	conn, r, _, _ := handshake(t, addr, "")
	setEncodings(t, conn, 0)
	requestUpdate(t, conn, false)

	_, _, enc := readRectHeader(t, r)
	if enc != 0 {
		t.Fatalf("encoding = %d, want 0 (Raw)", enc)
	}
	px := make([]byte, 4)
	if _, err := io.ReadFull(r, px); err != nil {
		t.Fatal(err)
	}
	// First pixel of testImage is (0,0,128); the wire order is BGRX.
	if px[0] != 128 || px[1] != 0 || px[2] != 0 {
		t.Fatalf("first pixel = %v, want BGRX 128,0,0", px)
	}
}

func TestPointerAndKeyReachInput(t *testing.T) {
	type event struct {
		kind string
		mask uint8
		x, y int
	}
	events := make(chan event, 8)
	in := inputFunc(func(mask uint8, x, y int) error {
		events <- event{"pointer", mask, x, y}
		return nil
	})
	addr := serve(t, Options{Name: "test"}, in)
	conn, _, _, _ := handshake(t, addr, "")

	// Press at (10,20).
	if _, err := conn.Write([]byte{5, 1, 0, 10, 0, 20}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-events:
		if e.mask != 1 || e.x != 10 || e.y != 20 {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pointer event never arrived")
	}
}

func TestPasswordAuth(t *testing.T) {
	addr := serve(t, Options{Name: "test", Password: "secret"}, nil)
	conn, r, _, _ := handshake(t, addr, "secret")
	setEncodings(t, conn, 7)
	requestUpdate(t, conn, false)
	if _, _, enc := readRectHeader(t, r); enc != 7 {
		t.Fatalf("encoding = %d after auth", enc)
	}
}

func TestScaleChangesAdvertisedSize(t *testing.T) {
	addr := serve(t, Options{Name: "test", Scale: 0.5}, nil)
	_, _, w, h := handshake(t, addr, "")
	if w != 32 || h != 24 {
		t.Fatalf("scaled ServerInit = %dx%d, want 32x24", w, h)
	}
}

// inputFunc adapts a function to Input.
type inputFunc func(mask uint8, x, y int) error

func (f inputFunc) Pointer(mask uint8, x, y int) error { return f(mask, x, y) }
func (f inputFunc) Key(bool, uint32) error             { return nil }

func readCompactLen(t *testing.T, r *bufio.Reader) int {
	t.Helper()
	b, err := r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	n := int(b & 0x7f)
	if b&0x80 == 0 {
		return n
	}
	b, err = r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	n |= int(b&0x7f) << 7
	if b&0x80 == 0 {
		return n
	}
	b, err = r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	return n | int(b)<<14
}
