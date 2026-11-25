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
	// OnReady runs once the upstream device has authenticated and sent
	// ServerInit. It is how the inventory learns a device is genuinely online
	// without a separate probe or health-check connection.
	OnReady func()
}

// Proxy opens the device connection, terminates just the RFB handshake, and
// then copies bytes in both directions.
//
// Ordering matters for the device's blacklist: the downstream client speaks
// first (RFB always starts with the server banner, but the client version
// arrives immediately after), and the hub does not connect to the device until
// it has a version and, for token sessions, a valid token. A browser tab that
// disappears before negotiating therefore never produces an unauthenticated
// connection that TigerVNC would count as a failure.
func Proxy(ctx context.Context, down net.Conn, opt Options) error {
	if opt.Timeout <= 0 {
		opt.Timeout = 5 * time.Second
	}
	defer down.Close()
	if tc, ok := down.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	_ = down.SetDeadline(time.Now().Add(opt.Timeout))
	dIn := bufio.NewReader(down)

	// 1. Downstream banner + version. Send it immediately; no silent TCP
	// connection is left waiting for a client that already went away.
	if _, err := io.WriteString(down, "RFB 003.008\n"); err != nil {
		return &DownstreamAuthError{Err: fmt.Errorf("write downstream banner: %w", err)}
	}
	downBanner, err := readBanner(dIn)
	if err != nil {
		return &DownstreamAuthError{Err: fmt.Errorf("read client banner: %w", err)}
	}
	if major, minor, err := parseVersion(downBanner); err != nil || major != 3 || minor < 8 {
		return &DownstreamAuthError{Err: fmt.Errorf("client must speak RFB 3.8, got %q", downBanner)}
	}

	// 2. Downstream security negotiation up to (but not including) the final
	// SecurityResult. A bad hub token is rejected here, before any packet goes
	// to the device.
	ds, err := beginDownstreamAuth(dIn, down, opt.DownstreamAuth, opt.Token)
	if err != nil {
		if IsDownstreamAuthFailure(err) {
			return err
		}
		return &DownstreamAuthError{Err: err}
	}

	// 3. Only now does the hub touch the device.
	dialer := net.Dialer{Timeout: opt.Timeout}
	up, err := dialer.DialContext(ctx, "tcp", opt.UpstreamAddr)
	if err != nil {
		ds.fail("could not connect to device")
		return fmt.Errorf("connect to device: %w", err)
	}
	defer up.Close()
	if tc, ok := up.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	deadline := time.Now().Add(opt.Timeout)
	_ = down.SetDeadline(deadline)
	_ = up.SetDeadline(deadline)
	uIn := bufio.NewReader(up)

	upBanner, err := readBanner(uIn)
	if err != nil {
		ds.fail("device did not send an RFB banner")
		return fmt.Errorf("read device banner: %w", err)
	}
	upVer, err := upstreamVersion(upBanner)
	if err != nil {
		ds.fail("device speaks an unsupported RFB version")
		return err
	}
	if _, err := io.WriteString(up, upVer); err != nil {
		ds.fail("device closed during RFB negotiation")
		return fmt.Errorf("write device version: %w", err)
	}

	// 4. Authenticate to the device. On failure, tell the downstream client the
	// real reason instead of dropping the socket; noVNC can then show
	// "Too many security failures" rather than a generic "failed to connect".
	if _, _, err := negotiateUpstream(uIn, up, upVer, opt.UpstreamPass, opt.UpstreamUser, opt.UpstreamARD); err != nil {
		ds.fail(upstreamFailureReason(err))
		return fmt.Errorf("upstream authentication: %w", err)
	}
	if err := ds.success(); err != nil {
		return err
	}

	// 5. ClientInit (shared flag) flows unchanged to the device.
	shared, err := dIn.ReadByte()
	if err != nil {
		return fmt.Errorf("read ClientInit: %w", err)
	}
	if err := writeByte(up, shared); err != nil {
		return fmt.Errorf("write ClientInit: %w", err)
	}

	// 6. ServerInit is 24 fixed bytes followed by the desktop name.
	si, err := readServerInit(uIn)
	if err != nil {
		return err
	}
	// One write per RFB message: over WebSocket each write becomes one frame,
	// and websockify-era clients (py-wsvnc, older noVNC forks) expect a
	// message not to be split across frames.
	if _, err := down.Write(append(append([]byte{}, si.Header...), si.DesktopName...)); err != nil {
		return fmt.Errorf("write ServerInit: %w", err)
	}

	_ = down.SetDeadline(time.Time{})
	_ = up.SetDeadline(time.Time{})
	if opt.OnReady != nil {
		opt.OnReady()
	}

	pipe(&readerConn{Conn: down, r: dIn}, &readerConn{Conn: up, r: uIn})
	return nil
}

// downstreamAuth holds a security negotiation whose result is deliberately
// delayed until the upstream device has authenticated. The downstream client is
// already waiting; sending success early would leave no way to report an
// upstream rejection.
type downstreamAuth struct {
	out  io.Writer
	done bool
}

func beginDownstreamAuth(in *bufio.Reader, out io.Writer, mode AuthMode, token string) (*downstreamAuth, error) {
	typ := byte(1)
	if mode == AuthVNC {
		typ = 2
		if len(token) < 8 {
			return nil, &DownstreamAuthError{Err: errors.New("hub token must be 8 characters")}
		}
	}
	if _, err := out.Write([]byte{1, typ}); err != nil { // one security type, in one frame
		return nil, err
	}
	selected, err := in.ReadByte()
	if err != nil {
		return nil, err
	}
	if selected != typ {
		return nil, &DownstreamAuthError{Err: fmt.Errorf("client selected security type %d, expected %d", selected, typ)}
	}

	if typ == 2 {
		challenge := make([]byte, 16)
		if _, err := rand.Read(challenge); err != nil {
			return nil, err
		}
		if _, err := out.Write(challenge); err != nil {
			return nil, err
		}
		resp := make([]byte, 16)
		if _, err := io.ReadFull(in, resp); err != nil {
			return nil, err
		}
		want := Response(challenge, token)
		if want == nil || subtle.ConstantTimeCompare(resp, want) != 1 {
			_ = writeFailure(out, "invalid hub token")
			return nil, &DownstreamAuthError{Err: errors.New("invalid hub token")}
		}
	}
	return &downstreamAuth{out: out}, nil
}

func (d *downstreamAuth) success() error {
	if d == nil || d.done {
		return nil
	}
	d.done = true
	return binary.Write(d.out, binary.BigEndian, uint32(0))
}

func (d *downstreamAuth) fail(reason string) {
	if d == nil || d.done {
		return
	}
	d.done = true
	if strings.TrimSpace(reason) == "" {
		reason = "authentication failed"
	}
	if len(reason) > 1024 {
		reason = reason[:1024]
	}
	_ = writeFailure(d.out, reason)
}

// writeFailure sends a failed SecurityResult and its reason as one message.
func writeFailure(w io.Writer, reason string) error {
	buf := binary.BigEndian.AppendUint32(nil, 1)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(reason)))
	_, err := w.Write(append(buf, reason...))
	return err
}

func upstreamFailureReason(err error) string {
	if IsTooManySecurityFailures(err) {
		return TooManySecurityFailuresHint
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "device authentication failed"
	}
	return "device authentication failed: " + msg
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

// negotiateUpstream authenticates to the device and returns the security types
// it offered plus the one that was selected. The returned values drive the
// device-card warning and the "is this device usable?" check.
func negotiateUpstream(in *bufio.Reader, out io.Writer, ver, password, username string, useARD bool) ([]byte, byte, error) {
	var types []byte
	if ver == "RFB 003.003\n" {
		var t uint32
		if err := binary.Read(in, binary.BigEndian, &t); err != nil {
			return nil, 0, err
		}
		if t == 0 {
			return nil, 0, readReason(in)
		}
		if t > 255 {
			return nil, 0, fmt.Errorf("invalid security type %d", t)
		}
		types = []byte{byte(t)}
	} else {
		n, err := in.ReadByte()
		if err != nil {
			return nil, 0, err
		}
		if n == 0 {
			return nil, 0, readReason(in)
		}
		types = make([]byte, int(n))
		if _, err := io.ReadFull(in, types); err != nil {
			return nil, 0, err
		}
	}

	var chosen byte
	switch {
	case useARD:
		if !bytes.Contains(types, []byte{securityTypeARD}) {
			return types, 0, fmt.Errorf("device offers no macOS account authentication (type 30; offered %v)", types)
		}
		chosen = securityTypeARD
	case password != "" && bytes.Contains(types, []byte{2}):
		chosen = 2
	case bytes.Contains(types, []byte{1}):
		chosen = 1
	}
	if chosen == 0 {
		return types, 0, fmt.Errorf("device offers no supported security type (offered %v)", types)
	}
	if ver != "RFB 003.003\n" {
		if err := writeByte(out, chosen); err != nil {
			return types, chosen, err
		}
	}

	switch chosen {
	case 2:
		challenge := make([]byte, 16)
		if _, err := io.ReadFull(in, challenge); err != nil {
			return types, chosen, err
		}
		resp := Response(challenge, password)
		if resp == nil {
			return types, chosen, errors.New("could not compute VNC authentication response")
		}
		if _, err := out.Write(resp); err != nil {
			return types, chosen, err
		}
	case securityTypeARD:
		if err := ardAuth(in, out, username, password); err != nil {
			return types, chosen, err
		}
	}

	// 3.8 sends SecurityResult for None as well; older versions only for VNC
	// auth.
	if chosen == 2 || chosen == securityTypeARD || ver == "RFB 003.008\n" {
		var result uint32
		if err := binary.Read(in, binary.BigEndian, &result); err != nil {
			return types, chosen, err
		}
		if result != 0 {
			return types, chosen, readReason(in)
		}
	}
	return types, chosen, nil
}

func readReason(r *bufio.Reader) error {
	var n uint32
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return &SecurityError{Reason: "authentication failed (no reason given)"}
	}
	if n > 4096 {
		n = 4096
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return &SecurityError{Reason: "authentication failed (truncated reason)"}
	}
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		msg = "authentication failed"
	}
	return &SecurityError{Reason: msg}
}

func writeByte(w io.Writer, b byte) error {
	var buf [1]byte
	buf[0] = b
	_, err := w.Write(buf[:])
	return err
}

type serverInit struct {
	Header      []byte
	Width       uint16
	Height      uint16
	DesktopName string
}

func readServerInit(in *bufio.Reader) (serverInit, error) {
	hdr := make([]byte, 24)
	if _, err := io.ReadFull(in, hdr); err != nil {
		return serverInit{}, fmt.Errorf("read ServerInit: %w", err)
	}
	nameLen := binary.BigEndian.Uint32(hdr[20:24])
	if nameLen > 64<<10 {
		return serverInit{}, fmt.Errorf("desktop name too long: %d", nameLen)
	}
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(in, name); err != nil {
		return serverInit{}, fmt.Errorf("read desktop name: %w", err)
	}
	return serverInit{
		Header:      hdr,
		Width:       binary.BigEndian.Uint16(hdr[0:2]),
		Height:      binary.BigEndian.Uint16(hdr[2:4]),
		DesktopName: string(name),
	}, nil
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
