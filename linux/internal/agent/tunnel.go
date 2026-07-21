package agent

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The binary tunnel lane, mirroring the macOS Tunnel and the framing in the
// server's protocol/stream.go. The server opens a stream (Open with a
// "host:port"); this dials that TCP target and pipes bytes both ways. Frames:
//
//	[type:1][stream id:8 BE][payload]   type 1=Open 2=Data 3=Close
//
// Streams are only ever opened from the server side; the device just answers.
const (
	frameOpen  byte = 1
	frameData  byte = 2
	frameClose byte = 3
)

const streamHeaderLen = 1 + 8

// Tunnel manages the device's answer side of the TCP tunnel lane.
type Tunnel struct {
	send  func([]byte) // sends a binary frame back over the WebSocket
	mu    sync.Mutex
	conns map[uint64]net.Conn
}

func newTunnel(send func([]byte)) *Tunnel {
	return &Tunnel{send: send, conns: make(map[uint64]net.Conn)}
}

// handle processes one inbound binary frame from the server.
func (t *Tunnel) handle(frame []byte) {
	if len(frame) < streamHeaderLen {
		return
	}
	typ := frame[0]
	id := binary.BigEndian.Uint64(frame[1:streamHeaderLen])
	payload := frame[streamHeaderLen:]
	switch typ {
	case frameOpen:
		t.open(id, string(payload))
	case frameData:
		t.mu.Lock()
		conn := t.conns[id]
		t.mu.Unlock()
		if conn != nil {
			_, _ = conn.Write(payload)
		}
	case frameClose:
		t.mu.Lock()
		conn, ok := t.conns[id]
		if ok {
			delete(t.conns, id)
		}
		t.mu.Unlock()
		if ok {
			_ = conn.Close()
		}
	}
}

func (t *Tunnel) open(id uint64, target string) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		t.emitClose(id, "bad target "+target)
		return
	}
	// Refuse targets with clear SSRF value: the cloud metadata endpoint
	// (169.254.169.254) and other link-local / unspecified / multicast
	// addresses. Loopback and private ranges stay allowed — reaching this host's
	// own services and LAN is the point. The server enforces the same policy;
	// this is device-side defense in depth.
	if isBlockedTargetHost(host) {
		t.emitClose(id, "target "+host+" is not an allowed address")
		return
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10*time.Second)
	if err != nil {
		t.emitClose(id, err.Error())
		return
	}
	t.mu.Lock()
	t.conns[id] = conn
	t.mu.Unlock()
	go t.readLoop(id, conn)
}

func (t *Tunnel) readLoop(id uint64, conn net.Conn) {
	buf := make([]byte, 64*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			t.send(encodeFrame(frameData, id, buf[:n]))
		}
		if err != nil {
			t.emitClose(id, "")
			return
		}
	}
}

// emitClose tears the stream down locally and tells the server it closed (empty
// reason = EOF). It only emits when it actually owned the stream, so a
// server-initiated close doesn't bounce a duplicate frame back.
func (t *Tunnel) emitClose(id uint64, reason string) {
	t.mu.Lock()
	conn, ok := t.conns[id]
	if ok {
		delete(t.conns, id)
	}
	t.mu.Unlock()
	// A dial that never registered a conn (bad target / dial error) still reports
	// the failure once; a stream already removed by the server stays quiet.
	if !ok && reason == "" {
		return
	}
	if conn != nil {
		_ = conn.Close()
	}
	t.send(encodeFrame(frameClose, id, []byte(reason)))
}

func (t *Tunnel) closeAll() {
	t.mu.Lock()
	for _, c := range t.conns {
		_ = c.Close()
	}
	t.conns = make(map[uint64]net.Conn)
	t.mu.Unlock()
}

func encodeFrame(typ byte, id uint64, payload []byte) []byte {
	b := make([]byte, streamHeaderLen+len(payload))
	b[0] = typ
	binary.BigEndian.PutUint64(b[1:streamHeaderLen], id)
	copy(b[streamHeaderLen:], payload)
	return b
}

// isBlockedTargetHost is a best-effort SSRF guard: block link-local (incl. the
// 169.254.169.254 metadata endpoint), unspecified, and multicast literals.
// Numeric range checks apply only to real IPv4 literals so a hostname like
// "224.example.com" isn't flagged. Loopback and private ranges are allowed.
func isBlockedTargetHost(host string) bool {
	h := strings.ToLower(host)
	// IPv6: unspecified, link-local (fe80::/10), multicast (ff00::/8).
	if h == "::" || strings.HasPrefix(h, "fe80:") || (strings.HasPrefix(h, "ff") && strings.Contains(h, ":")) {
		return true
	}
	// IPv4: only judge genuine dotted-quad literals.
	parts := strings.Split(h, ".")
	if len(parts) == 4 {
		var octets [4]int
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 || n > 255 {
				return false
			}
			octets[i] = n
		}
		if octets == [4]int{0, 0, 0, 0} {
			return true // unspecified
		}
		if octets[0] == 169 && octets[1] == 254 {
			return true // link-local incl. metadata
		}
		if octets[0] >= 224 && octets[0] <= 239 {
			return true // multicast
		}
	}
	return false
}
