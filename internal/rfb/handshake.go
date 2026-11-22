package rfb

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AuthMode is what the downstream client must do before it is allowed through.
type AuthMode int

const (
	// AuthNone is for browser sessions that already passed the hub's cookie
	// authentication. The browser never sees the device password.
	AuthNone AuthMode = iota
	// AuthVNC asks the downstream client for the per-device hub token.
	AuthVNC
)

// Options configures one proxied RFB connection.
type Options struct {
	UpstreamAddr   string
	UpstreamPass   string // the real device password; never sent downstream
	UpstreamUser   string // macOS account name for ARD authentication
	UpstreamARD    bool   // use Apple DH (security type 30) instead of a VNC password
	DownstreamAuth AuthMode
	Token          string // 8 characters when DownstreamAuth is AuthVNC
	Timeout        time.Duration
}

// Proxy connects to UpstreamAddr, terminates just the RFB handshake, and then
// copies bytes in both directions. It deliberately does not decode
// framebuffer updates or synthesize input: after ServerInit it is a blind pipe.
func Proxy(ctx context.Context, down net.Conn, opt Options) error {
	if opt.Timeout <= 0 {
		opt.Timeout = 5 * time.Second
	}
	d := net.Dialer{Timeout: opt.Timeout}
	up, err := d.DialContext(ctx, "tcp", opt.UpstreamAddr)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}
	defer down.Close()
	defer up.Close()
	if tc, ok := up.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	if tc, ok := down.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	deadline := time.Now().Add(opt.Timeout)
	_ = down.SetDeadline(deadline)
	_ = up.SetDeadline(deadline)

	dIn := bufio.NewReader(down)
	uIn := bufio.NewReader(up)
	if err := handshake(dIn, down, uIn, up, opt); err != nil {
		return err
	}
	_ = down.SetDeadline(time.Time{})
	_ = up.SetDeadline(time.Time{})

	dc := &readerConn{Conn: down, r: dIn}
	uc := &readerConn{Conn: up, r: uIn}
	pipe(dc, uc)
	return nil
}

func handshake(dIn *bufio.Reader, dOut io.Writer, uIn *bufio.Reader, uOut io.Writer, opt Options) error {
	upBanner, err := readBanner(uIn)
	if err != nil {
		return fmt.Errorf("read device banner: %w", err)
	}
	upVer, err := upstreamVersion(upBanner)
	if err != nil {
		return err
	}

	if _, err := io.WriteString(dOut, "RFB 003.008\n"); err != nil {
		return fmt.Errorf("write downstream banner: %w", err)
	}
	downBanner, err := readBanner(dIn)
	if err != nil {
		return fmt.Errorf("read client banner: %w", err)
	}
	if major, minor, err := parseVersion(downBanner); err != nil || major != 3 || minor < 8 {
		return fmt.Errorf("client must speak RFB 3.8, got %q", downBanner)
	}
	if _, err := io.WriteString(uOut, upVer); err != nil {
		return fmt.Errorf("write device version: %w", err)
	}

	if err := negotiateUpstream(uIn, uOut, upVer, opt.UpstreamPass, opt.UpstreamUser, opt.UpstreamARD); err != nil {
		return fmt.Errorf("upstream authentication: %w", err)
	}
	if err := negotiateDownstream(dIn, dOut, opt.DownstreamAuth, opt.Token); err != nil {
		return fmt.Errorf("downstream authentication: %w", err)
	}

	// ClientInit (shared flag) flows unchanged to the device.
	shared, err := dIn.ReadByte()
	if err != nil {
		return fmt.Errorf("read ClientInit: %w", err)
	}
	if err := writeByte(uOut, shared); err != nil {
		return fmt.Errorf("write ClientInit: %w", err)
	}

	// ServerInit is 24 fixed bytes followed by the desktop name.
	hdr := make([]byte, 24)
	if _, err := io.ReadFull(uIn, hdr); err != nil {
		return fmt.Errorf("read ServerInit: %w", err)
	}
	nameLen := binary.BigEndian.Uint32(hdr[20:24])
	if nameLen > 64<<10 {
		return fmt.Errorf("desktop name too long: %d", nameLen)
	}
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(uIn, name); err != nil {
		return fmt.Errorf("read desktop name: %w", err)
	}
	if _, err := dOut.Write(hdr); err != nil {
		return fmt.Errorf("write ServerInit: %w", err)
	}
	if _, err := dOut.Write(name); err != nil {
		return fmt.Errorf("write desktop name: %w", err)
	}
	return nil
}

func readBanner(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
		return "", err
	}
	if len(line) > 64 {
		return "", fmt.Errorf("banner too long")
	}
	return strings.TrimSpace(line), nil
}

func parseVersion(banner string) (int, int, error) {
	fields := strings.Fields(banner)
	if len(fields) != 2 || fields[0] != "RFB" {
		return 0, 0, fmt.Errorf("invalid RFB banner %q", banner)
	}
	parts := strings.SplitN(fields[1], ".", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid RFB version %q", fields[1])
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid RFB major %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid RFB minor %q", parts[1])
	}
	return major, minor, nil
}

func upstreamVersion(banner string) (string, error) {
	major, minor, err := parseVersion(banner)
	if err != nil {
		return "", err
	}
	if major != 3 {
		return "", fmt.Errorf("unsupported RFB major version %d", major)
	}
	switch {
	case minor >= 8:
		// Apple advertises odd minors (for example 003.889); 3.8 is the
		// interoperable floor for the handshake we implement.
		return "RFB 003.008\n", nil
	case minor == 7:
		return "RFB 003.007\n", nil
	case minor == 3:
		return "RFB 003.003\n", nil
	default:
		return "", fmt.Errorf("unsupported RFB version 3.%d", minor)
	}
}

func negotiateUpstream(in *bufio.Reader, out io.Writer, ver, password, username string, useARD bool) error {
	var types []byte
	if ver == "RFB 003.003\n" {
		var t uint32
		if err := binary.Read(in, binary.BigEndian, &t); err != nil {
			return err
		}
		if t == 0 {
			return readReason(in)
		}
		if t > 255 {
			return fmt.Errorf("invalid security type %d", t)
		}
		types = []byte{byte(t)}
	} else {
		n, err := in.ReadByte()
		if err != nil {
			return err
		}
		if n == 0 {
			return readReason(in)
		}
		types = make([]byte, int(n))
		if _, err := io.ReadFull(in, types); err != nil {
			return err
		}
	}

	var chosen byte
	switch {
	case useARD:
		if !bytes.Contains(types, []byte{securityTypeARD}) {
			return fmt.Errorf("device offers no macOS account authentication (type 30; offered %v)", types)
		}
		chosen = securityTypeARD
	case password != "" && bytes.Contains(types, []byte{2}):
		chosen = 2
	case bytes.Contains(types, []byte{1}):
		chosen = 1
	}
	if chosen == 0 {
		return fmt.Errorf("device offers no supported security type (offered %v)", types)
	}
	if ver != "RFB 003.003\n" {
		if err := writeByte(out, chosen); err != nil {
			return err
		}
	}

	switch chosen {
	case 2:
		challenge := make([]byte, 16)
		if _, err := io.ReadFull(in, challenge); err != nil {
			return err
		}
		resp := Response(challenge, password)
		if resp == nil {
			return errors.New("could not compute VNC authentication response")
		}
		if _, err := out.Write(resp); err != nil {
			return err
		}
	case securityTypeARD:
		if err := ardAuth(in, out, username, password); err != nil {
			return err
		}
	}

	// 3.8 sends SecurityResult for None as well; older versions only for VNC
	// auth.
	if chosen == 2 || chosen == securityTypeARD || ver == "RFB 003.008\n" {
		var result uint32
		if err := binary.Read(in, binary.BigEndian, &result); err != nil {
			return err
		}
		if result != 0 {
			return readReason(in)
		}
	}
	return nil
}

func negotiateDownstream(in *bufio.Reader, out io.Writer, mode AuthMode, token string) error {
	typ := byte(1)
	if mode == AuthVNC {
		typ = 2
		if len(token) < 8 {
			return errors.New("hub token must be 8 characters")
		}
	}
	if err := writeByte(out, 1); err != nil {
		return err
	}
	if err := writeByte(out, typ); err != nil {
		return err
	}
	selected, err := in.ReadByte()
	if err != nil {
		return err
	}
	if selected != typ {
		return fmt.Errorf("client selected security type %d, expected %d", selected, typ)
	}

	if typ == 2 {
		challenge := make([]byte, 16)
		if _, err := rand.Read(challenge); err != nil {
			return err
		}
		if _, err := out.Write(challenge); err != nil {
			return err
		}
		resp := make([]byte, 16)
		if _, err := io.ReadFull(in, resp); err != nil {
			return err
		}
		want := Response(challenge, token)
		if want == nil || subtle.ConstantTimeCompare(resp, want) != 1 {
			_ = binary.Write(out, binary.BigEndian, uint32(1))
			_ = writeReason(out, "invalid hub token")
			return errors.New("invalid hub token")
		}
	}
	return binary.Write(out, binary.BigEndian, uint32(0))
}

func readReason(r *bufio.Reader) error {
	var n uint32
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return errors.New("authentication failed (no reason given)")
	}
	if n > 4096 {
		n = 4096
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return errors.New("authentication failed (truncated reason)")
	}
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		msg = "authentication failed"
	}
	return errors.New(msg)
}

func writeReason(w io.Writer, msg string) error {
	if err := binary.Write(w, binary.BigEndian, uint32(len(msg))); err != nil {
		return err
	}
	_, err := io.WriteString(w, msg)
	return err
}

func writeByte(w io.Writer, b byte) error {
	var buf [1]byte
	buf[0] = b
	_, err := w.Write(buf[:])
	return err
}

type readerConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *readerConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func pipe(a, b net.Conn) {
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			_ = a.Close()
			_ = b.Close()
		})
	}
	go func() {
		_, _ = io.Copy(a, b)
		closeBoth()
	}()
	_, _ = io.Copy(b, a)
	closeBoth()
}
