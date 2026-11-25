// Package rfbserver serves one RFB 3.8 screen over TCP.
//
// It is deliberately small: a single framebuffer, Raw and Tight/JPEG encodings,
// and pointer/keyboard input handed to an Input. Anything the protocol allows
// but a browser viewer never uses (cursor shapes, clipboard formats other than
// UTF-8, continuous updates) is either ignored or refused explicitly instead of
// being half-implemented.
package rfbserver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/des"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"net"
	"time"
)

// Encoding numbers from the RFB registry.
const (
	encRaw   int32 = 0
	encTight int32 = 7
)

// Frame is one captured screen image.
type Frame struct {
	Image image.Image
	Seq   uint64
}

// Source produces the device's current screen. Wait blocks until a frame newer
// than seq exists, so a client that asks for a non-incremental update gets one
// without the server busy-polling the device.
type Source interface {
	// Size reports the framebuffer size in device pixels.
	Size() (w, h int)
	// Wait returns a frame with Seq > after, or ctx.Err().
	Wait(ctx context.Context, after uint64) (Frame, error)
}

// Input receives what the viewer does.
type Input interface {
	// Pointer reports the current button mask at device pixel coordinates.
	Pointer(mask uint8, x, y int) error
	// Key reports a keysym press or release.
	Key(down bool, keysym uint32) error
}

// Options configure a server.
type Options struct {
	// Name is the desktop name the viewer shows.
	Name string
	// Password enables classic VNC authentication when non-empty. RFB carries
	// only the first 8 characters.
	Password string
	// Scale is the output scale (0 < Scale <= 1). Zero means 1.
	Scale float64
	// Quality is the Tight/JPEG quality, 1..100. Zero means 70.
	Quality int
	// Logf receives one line per notable event. Nil discards.
	Logf func(format string, args ...any)
}

// Server serves RFB connections from one Source.
type Server struct {
	opts   Options
	source Source
	input  Input
}

// New returns a server. A nil input is legal and makes the session view-only.
func New(source Source, input Input, opts Options) *Server {
	if opts.Scale <= 0 {
		opts.Scale = 1
	}
	if opts.Quality <= 0 {
		opts.Quality = 70
	}
	if opts.Name == "" {
		opts.Name = "iosvnc"
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Server{source: source, input: input, opts: opts}
}

// OutputSize is the framebuffer size clients will be told about.
func (s *Server) OutputSize() (int, int) {
	w, h := s.source.Size()
	return OutputSize(w, h, s.opts.Scale)
}

// OutputSize applies a scale factor to a native framebuffer size. A viewer's
// pointer coordinates are in this space, so callers that also inject input need
// the same numbers the server advertises.
func OutputSize(w, h int, scale float64) (int, int) {
	if scale >= 1 {
		return w, h
	}
	ow := int(float64(w)*scale + 0.5)
	oh := int(float64(h)*scale + 0.5)
	if ow < 1 {
		ow = 1
	}
	if oh < 1 {
		oh = 1
	}
	return ow, oh
}

// ServeConn runs one viewer session until the connection ends.
func (s *Server) ServeConn(conn net.Conn) error {
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	c := &client{
		conn:  conn,
		r:     bufio.NewReaderSize(conn, 32*1024),
		w:     bufio.NewWriterSize(conn, 256*1024),
		start: time.Now(),
	}
	defer func() {
		if c.frames == 0 {
			return
		}
		elapsed := time.Since(c.start).Seconds()
		if elapsed <= 0 {
			elapsed = 1
		}
		// A frame rate needs a few frames to mean anything.
		if c.frames < 5 {
			s.opts.Logf("session: %d frames, %.1f MB", c.frames, float64(c.bytes)/1e6)
			return
		}
		s.opts.Logf("session: %d frames, %.1f MB, %.1f fps",
			c.frames, float64(c.bytes)/1e6, float64(c.frames)/elapsed)
	}()
	if err := s.handshake(c); err != nil {
		return err
	}
	return s.loop(c)
}

// client holds per-connection protocol state.
type client struct {
	conn     net.Conn
	r        *bufio.Reader
	w        *bufio.Writer
	encTight bool
	lastSeq  uint64
	ptrMask  uint8
	ptrX     int
	ptrY     int

	frames int
	bytes  int64
	start  time.Time
	warned bool
}

func (s *Server) handshake(c *client) error {
	if err := writeVersion(c.w); err != nil {
		return err
	}
	clientMajor, clientMinor, err := readVersion(c.r)
	if err != nil {
		return err
	}
	if clientMajor != 3 || (clientMinor != 3 && clientMinor != 7 && clientMinor != 8) {
		return fmt.Errorf("unsupported RFB version %d.%d", clientMajor, clientMinor)
	}
	if err := s.security(c, clientMinor); err != nil {
		return err
	}

	// ClientInit: the shared flag. This server has one framebuffer and does not
	// kick other viewers, so shared and exclusive behave the same.
	if _, err := c.r.ReadByte(); err != nil {
		return fmt.Errorf("read ClientInit: %w", err)
	}

	width, height := s.OutputSize()
	if err := s.writeServerInit(c.w, width, height); err != nil {
		return err
	}
	s.opts.Logf("session started: %dx%d", width, height)
	return nil
}

func writeVersion(w *bufio.Writer) error {
	_, err := w.WriteString("RFB 003.008\n")
	if err != nil {
		return err
	}
	return w.Flush()
}

func readVersion(r *bufio.Reader) (int, int, error) {
	buf := make([]byte, 12)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, 0, fmt.Errorf("read version: %w", err)
	}
	var major, minor int
	if _, err := fmt.Sscanf(string(buf), "RFB %3d.%3d\n", &major, &minor); err != nil {
		return 0, 0, fmt.Errorf("parse version %q: %w", buf, err)
	}
	return major, minor, nil
}

func (s *Server) security(c *client, minor int) error {
	if minor == 3 {
		// 3.3 has no type list: the server names exactly one type.
		t := uint32(1)
		if s.opts.Password != "" {
			t = 2
		}
		if err := binary.Write(c.w, binary.BigEndian, t); err != nil {
			return err
		}
		if err := c.w.Flush(); err != nil {
			return err
		}
		if t == 2 {
			return s.vncAuth(c, minor)
		}
		return nil
	}

	// 3.7 and 3.8 send the list of supported types.
	types := []byte{1}
	if s.opts.Password != "" {
		types = []byte{2}
	}
	if err := c.w.WriteByte(byte(len(types))); err != nil {
		return err
	}
	if _, err := c.w.Write(types); err != nil {
		return err
	}
	if err := c.w.Flush(); err != nil {
		return err
	}

	chosen, err := c.r.ReadByte()
	if err != nil {
		return fmt.Errorf("read security type: %w", err)
	}
	if chosen == 0 {
		reason, _ := readReason(c.r)
		return fmt.Errorf("viewer refused security types: %s", reason)
	}
	switch chosen {
	case 1:
		// SecurityResult is sent for None from 3.8 on; 3.7 has no result here.
		if minor >= 8 {
			return writeSecurityResult(c.w, true)
		}
		return nil
	case 2:
		return s.vncAuth(c, minor)
	default:
		return fmt.Errorf("viewer chose unsupported security type %d", chosen)
	}
}

// vncAuth performs the classic challenge/response: DES-ECB with the password as
// key, every password byte bit-reversed, over a 16-byte challenge.
func (s *Server) vncAuth(c *client, minor int) error {
	challenge := make([]byte, 16)
	if _, err := rand.Read(challenge); err != nil {
		return err
	}
	if _, err := c.w.Write(challenge); err != nil {
		return err
	}
	if err := c.w.Flush(); err != nil {
		return err
	}

	response := make([]byte, 16)
	if _, err := io.ReadFull(c.r, response); err != nil {
		return fmt.Errorf("read auth response: %w", err)
	}

	want, err := vncResponse(s.opts.Password, challenge)
	if err != nil {
		return err
	}
	if !equal(want, response) {
		_ = writeSecurityResult(c.w, false)
		return errors.New("authentication failed")
	}
	if minor >= 8 {
		return writeSecurityResult(c.w, true)
	}
	// 3.3 sends the result only after VNC auth.
	return writeSecurityResult(c.w, true)
}

func vncResponse(password string, challenge []byte) ([]byte, error) {
	key := make([]byte, 8)
	copy(key, password)
	for i := range key {
		key[i] = reverseBits(key[i])
	}
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 16)
	block.Encrypt(out[0:8], challenge[0:8])
	block.Encrypt(out[8:16], challenge[8:16])
	return out, nil
}

func reverseBits(b byte) byte {
	var out byte
	for i := 0; i < 8; i++ {
		out <<= 1
		out |= b & 1
		b >>= 1
	}
	return out
}

func writeSecurityResult(w *bufio.Writer, ok bool) error {
	var v uint32
	if !ok {
		v = 1
	}
	if err := binary.Write(w, binary.BigEndian, v); err != nil {
		return err
	}
	if !ok {
		// A failure carries a reason string for 3.8 viewers.
		reason := []byte("authentication failed")
		if err := binary.Write(w, binary.BigEndian, uint32(len(reason))); err != nil {
			return err
		}
		if _, err := w.Write(reason); err != nil {
			return err
		}
	}
	return w.Flush()
}

func readReason(r *bufio.Reader) (string, error) {
	var n uint32
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return "", err
	}
	if n > 4096 {
		return "", fmt.Errorf("reason too long: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func (s *Server) writeServerInit(w *bufio.Writer, width, height int) error {
	hdr := make([]byte, 24)
	binary.BigEndian.PutUint16(hdr[0:2], uint16(width))
	binary.BigEndian.PutUint16(hdr[2:4], uint16(height))
	// PixelFormat: 32 bpp, depth 24, little-endian, true colour, 8 bits each,
	// red/green/blue at shifts 16/8/0, which puts the bytes in BGRX order.
	hdr[4] = 32
	hdr[5] = 24
	hdr[6] = 0
	hdr[7] = 1
	binary.BigEndian.PutUint16(hdr[8:10], 255)
	binary.BigEndian.PutUint16(hdr[10:12], 255)
	binary.BigEndian.PutUint16(hdr[12:14], 255)
	hdr[14] = 16
	hdr[15] = 8
	hdr[16] = 0
	name := []byte(s.opts.Name)
	binary.BigEndian.PutUint32(hdr[20:24], uint32(len(name)))
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if _, err := w.Write(name); err != nil {
		return err
	}
	return w.Flush()
}

// loop reads viewer messages until the connection closes.
func (s *Server) loop(c *client) error {
	for {
		t, err := c.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || isClosed(err) {
				return nil
			}
			return err
		}
		switch t {
		case 0: // SetPixelFormat: ignored, this server always sends BGRX.
			var buf [19]byte
			if _, err := io.ReadFull(c.r, buf[:]); err != nil {
				return err
			}
		case 2: // SetEncodings
			if err := s.readEncodings(c); err != nil {
				return err
			}
		case 3: // FramebufferUpdateRequest
			if err := s.sendUpdate(c); err != nil {
				return err
			}
		case 4: // KeyEvent
			var buf [7]byte
			if _, err := io.ReadFull(c.r, buf[:]); err != nil {
				return err
			}
			if s.input == nil {
				continue
			}
			down := buf[0] != 0
			keysym := binary.BigEndian.Uint32(buf[3:7])
			if err := s.input.Key(down, keysym); err != nil {
				s.opts.Logf("key: %v", err)
			}
		case 5: // PointerEvent
			var buf [5]byte
			if _, err := io.ReadFull(c.r, buf[:]); err != nil {
				return err
			}
			mask := buf[0]
			x := int(binary.BigEndian.Uint16(buf[1:3]))
			y := int(binary.BigEndian.Uint16(buf[3:5]))
			if s.input == nil {
				continue
			}
			if err := s.input.Pointer(mask, x, y); err != nil {
				s.opts.Logf("pointer: %v", err)
			}
		case 6: // ClientCutText
			if _, err := readReason(c.r); err != nil {
				return err
			}
		case 150: // EnableContinuousUpdates: pad 1 + 4 bytes, unsupported.
			var buf [5]byte
			if _, err := io.ReadFull(c.r, buf[:]); err != nil {
				return err
			}
		case 248: // ClientFence: pad 3 + payload length, ignored.
			var hdr [7]byte
			if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
				return err
			}
			n := binary.BigEndian.Uint32(hdr[3:7])
			if n > 1<<20 {
				return fmt.Errorf("fence payload too large: %d", n)
			}
			if _, err := io.CopyN(io.Discard, c.r, int64(n)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported client message type %d", t)
		}
	}
}

func (s *Server) readEncodings(c *client) error {
	var hdr [3]byte
	if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
		return err
	}
	n := int(binary.BigEndian.Uint16(hdr[1:3]))
	if n > 1024 {
		return fmt.Errorf("too many encodings: %d", n)
	}
	buf := make([]byte, n*4)
	if _, err := io.ReadFull(c.r, buf); err != nil {
		return err
	}
	c.encTight = false
	for i := 0; i < n; i++ {
		e := int32(binary.BigEndian.Uint32(buf[i*4 : i*4+4]))
		if e == encTight {
			c.encTight = true
		}
	}
	if !c.encTight && !c.warned {
		c.warned = true
		w, h := s.OutputSize()
		s.opts.Logf("viewer did not offer Tight; raw frames at %dx%d are %.1f MB each - consider -scale", w, h, float64(w*h*4)/1e6)
	}
	return nil
}

func (s *Server) sendUpdate(c *client) error {
	var hdr [9]byte
	if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
		return err
	}
	incremental := hdr[0] != 0

	after := uint64(0)
	if incremental {
		after = c.lastSeq
	}
	frame, err := s.source.Wait(context.Background(), after)
	if err != nil {
		return err
	}
	c.lastSeq = frame.Seq

	w, h := s.OutputSize()
	img := scaleTo(frame.Image, w, h)

	if _, err := c.w.Write([]byte{0, 0, 0, 1}); err != nil { // Update, pad, 1 rect
		return err
	}
	var rect [12]byte
	binary.BigEndian.PutUint16(rect[0:2], 0)
	binary.BigEndian.PutUint16(rect[2:4], 0)
	binary.BigEndian.PutUint16(rect[4:6], uint16(w))
	binary.BigEndian.PutUint16(rect[6:8], uint16(h))
	if c.encTight {
		binary.BigEndian.PutUint32(rect[8:12], uint32(encTight))
		if _, err := c.w.Write(rect[:]); err != nil {
			return err
		}
		n, err := writeTightJPEG(c.w, img, s.opts.Quality)
		if err != nil {
			return err
		}
		c.frames++
		c.bytes += int64(n)
	} else {
		if !c.warned {
			c.warned = true
			s.opts.Logf("viewer did not offer Tight; raw frames at %dx%d are %.1f MB each - consider -scale", w, h, float64(w*h*4)/1e6)
		}
		binary.BigEndian.PutUint32(rect[8:12], uint32(encRaw))
		if _, err := c.w.Write(rect[:]); err != nil {
			return err
		}
		n, err := writeRawBGRX(c.w, img)
		if err != nil {
			return err
		}
		c.frames++
		c.bytes += int64(n)
	}
	return c.w.Flush()
}

// writeTightJPEG emits one Tight rectangle carrying a JPEG image: the control
// byte 0x90 (high nibble 9 = JPEG, no zlib stream resets), the compact length,
// then the JPEG bytes.
func writeTightJPEG(w *bufio.Writer, img *image.RGBA, quality int) (int, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return 0, err
	}
	if err := w.WriteByte(0x90); err != nil {
		return 0, err
	}
	if err := writeCompactLen(w, buf.Len()); err != nil {
		return 0, err
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return buf.Len(), nil
}

func writeCompactLen(w *bufio.Writer, n int) error {
	if n > 0x3FFFFF {
		return fmt.Errorf("tight rect too large: %d", n)
	}
	var b [3]byte
	b[0] = byte(n & 0x7f)
	if n > 0x7f {
		b[0] |= 0x80
		b[1] = byte((n >> 7) & 0x7f)
		if n > 0x3fff {
			b[1] |= 0x80
			b[2] = byte((n >> 14) & 0x7f)
		}
	}
	count := 1
	if n > 0x7f {
		count = 2
	}
	if n > 0x3fff {
		count = 3
	}
	_, err := w.Write(b[:count])
	return err
}

// writeRawBGRX writes the framebuffer as 4-byte pixels in the format
// writeServerInit advertises: blue, green, red, pad.
func writeRawBGRX(w *bufio.Writer, img *image.RGBA) (int, error) {
	b := img.Bounds()
	line := make([]byte, 0, b.Dx()*4)
	pix := img.Pix
	stride := img.Stride
	for y := b.Min.Y; y < b.Max.Y; y++ {
		line = line[:0]
		row := pix[(y-b.Min.Y)*stride:]
		for x := 0; x < b.Dx(); x++ {
			p := row[x*4 : x*4+4]
			line = append(line, p[2], p[1], p[0], 0)
		}
		if _, err := w.Write(line); err != nil {
			return 0, err
		}
	}
	return b.Dx() * b.Dy() * 4, nil
}

// scaleTo converts any image to an RGBA of exactly w x h, nearest-neighbour when
// the size changes.
func scaleTo(src image.Image, w, h int) *image.RGBA {
	sb := src.Bounds()
	if sb.Dx() == w && sb.Dy() == h {
		if rgba, ok := src.(*image.RGBA); ok && rgba.Bounds().Min == (image.Point{}) {
			return rgba
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if sb.Dx() == w && sb.Dy() == h {
		draw.Draw(dst, dst.Bounds(), src, sb.Min, draw.Src)
		return dst
	}
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + y*sb.Dy()/h
		for x := 0; x < w; x++ {
			sx := sb.Min.X + x*sb.Dx()/w
			r, g, b, a := src.At(sx, sy).RGBA()
			i := y*dst.Stride + x*4
			dst.Pix[i] = uint8(r >> 8)
			dst.Pix[i+1] = uint8(g >> 8)
			dst.Pix[i+2] = uint8(b >> 8)
			dst.Pix[i+3] = uint8(a >> 8)
		}
	}
	return dst
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isClosed(err error) bool {
	return errors.Is(err, net.ErrClosed)
}
