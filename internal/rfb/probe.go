package rfb

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// ProbeOptions configures an authenticated health probe.
type ProbeOptions struct {
	Addr     string
	Password string
	Username string // macOS account name when ARD is true
	ARD      bool
	Timeout  time.Duration
}

// ProbeResult is what a successful probe learned about the device.
type ProbeResult struct {
	Banner       string
	Version      string
	Types        []byte
	SecurityType byte
	Width        uint16
	Height       uint16
	DesktopName  string
}

// Probe performs one complete RFB handshake and authenticates with the stored
// device credential, then disconnects cleanly.
//
// This exists because there is no side-effect-free way to probe a VNC port.
// TigerVNC counts every connection that closes before authenticating as a
// security failure, and its counter does not decay, so a bare TCP banner read
// eventually blacklists the hub's IP. A probe that authenticates successfully
// is a normal client connection. Callers must still rate-limit it (see
// inventory's five-minute minimum) and bound consecutive failures so a wrong
// password cannot blacklist the hub either.
func Probe(ctx context.Context, opt ProbeOptions) (ProbeResult, error) {
	if opt.Timeout <= 0 {
		opt.Timeout = 3 * time.Second
	}
	dialer := net.Dialer{Timeout: opt.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", opt.Addr)
	if err != nil {
		return ProbeResult{}, err
	}
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	_ = conn.SetDeadline(time.Now().Add(opt.Timeout))
	in := bufio.NewReader(conn)

	banner, err := readBanner(in)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("read device banner: %w", err)
	}
	ver, err := upstreamVersion(banner)
	if err != nil {
		return ProbeResult{Banner: banner}, err
	}
	// Reply as soon as the server has spoken, before doing anything else.
	if _, err := io.WriteString(conn, ver); err != nil {
		return ProbeResult{Banner: banner, Version: ver}, fmt.Errorf("write device version: %w", err)
	}
	types, chosen, err := negotiateUpstream(in, conn, ver, opt.Password, opt.Username, opt.ARD)
	if err != nil {
		return ProbeResult{Banner: banner, Version: ver, Types: types, SecurityType: chosen}, &AuthError{Err: err}
	}

	// shared=1: never evict an existing user just because a health check ran.
	if err := writeByte(conn, 1); err != nil {
		return ProbeResult{Banner: banner, Version: ver, Types: types, SecurityType: chosen}, fmt.Errorf("write ClientInit: %w", err)
	}
	si, err := readServerInit(in)
	if err != nil {
		return ProbeResult{Banner: banner, Version: ver, Types: types, SecurityType: chosen}, err
	}
	return ProbeResult{
		Banner:       banner,
		Version:      ver,
		Types:        types,
		SecurityType: chosen,
		Width:        si.Width,
		Height:       si.Height,
		DesktopName:  si.DesktopName,
	}, nil
}

// SecurityTypes performs just enough of the RFB handshake to report which
// security types a server offers. It does not authenticate and does not select
// a type; it is a diagnostic for "why can't the hub connect?".
//
// A macOS Screen Sharing server offers Apple-specific types (for example 30,
// 33, 35, 36) next to the legacy VNC password when that is enabled. Type 30 is
// the account sign-in that reaches the logged-in console session; see D15.
func SecurityTypes(ctx context.Context, addr string, timeout time.Duration) (banner string, types []byte, err error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	in := bufio.NewReader(conn)

	banner, err = readBanner(in)
	if err != nil {
		return "", nil, err
	}
	ver, err := upstreamVersion(banner)
	if err != nil {
		return banner, nil, err
	}
	if _, err := io.WriteString(conn, ver); err != nil {
		return banner, nil, err
	}
	if ver == "RFB 003.003\n" {
		var t uint32
		if err := binary.Read(in, binary.BigEndian, &t); err != nil {
			return banner, nil, err
		}
		if t == 0 {
			return banner, nil, readReason(in)
		}
		return banner, []byte{byte(t)}, nil
	}
	n, err := in.ReadByte()
	if err != nil {
		return banner, nil, err
	}
	if n == 0 {
		return banner, nil, readReason(in)
	}
	types = make([]byte, int(n))
	if _, err := io.ReadFull(in, types); err != nil {
		return banner, nil, err
	}
	return banner, types, nil
}

// SupportsStandardAuth reports whether the password-based types (None or
// classic VNC auth) are present.
func SupportsStandardAuth(types []byte) bool {
	for _, t := range types {
		if t == 1 || t == 2 {
			return true
		}
	}
	return false
}

// SupportsARD reports whether Apple's type 30 account authentication is
// offered. It is present on macOS Screen Sharing and is the only type that
// signs the connection in as a specific console user.
func SupportsARD(types []byte) bool {
	for _, t := range types {
		if t == securityTypeARD {
			return true
		}
	}
	return false
}

// DescribeTypes is a small helper for logs and doctor output.
func DescribeTypes(types []byte) string {
	return fmt.Sprintf("%v", types)
}
