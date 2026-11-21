package gateway

import (
	"bytes"
	"context"
	"net"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// WSConn adapts a noVNC WebSocket to net.Conn. noVNC sends RFB bytes as binary
// messages; a single RFB read may span messages, hence the small read buffer.
type WSConn struct {
	c    *websocket.Conn
	ctx  context.Context
	mu   sync.Mutex // serializes writes
	rmu  sync.Mutex // serializes reads
	r    *bytes.Reader
	addr string
}

func NewWSConn(ctx context.Context, c *websocket.Conn) *WSConn {
	return &WSConn{c: c, ctx: ctx, addr: "websocket"}
}

func (w *WSConn) Read(p []byte) (int, error) {
	w.rmu.Lock()
	defer w.rmu.Unlock()
	for {
		if w.r != nil && w.r.Len() > 0 {
			return w.r.Read(p)
		}
		typ, data, err := w.c.Read(w.ctx)
		if err != nil {
			return 0, err
		}
		if typ != websocket.MessageBinary {
			continue
		}
		w.r = bytes.NewReader(data)
	}
}

func (w *WSConn) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.c.Write(w.ctx, websocket.MessageBinary, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *WSConn) Close() error {
	_ = w.c.Close(websocket.StatusNormalClosure, "closed")
	return nil
}

func (w *WSConn) LocalAddr() net.Addr                { return wsAddr{"local"} }
func (w *WSConn) RemoteAddr() net.Addr               { return wsAddr{w.addr} }
func (w *WSConn) SetDeadline(t time.Time) error      { return nil }
func (w *WSConn) SetReadDeadline(t time.Time) error  { return nil }
func (w *WSConn) SetWriteDeadline(t time.Time) error { return nil }

type wsAddr struct{ name string }

func (a wsAddr) Network() string { return "websocket" }
func (a wsAddr) String() string  { return a.name }
