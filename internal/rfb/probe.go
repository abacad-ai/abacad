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
