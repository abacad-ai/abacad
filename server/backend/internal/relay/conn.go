// Package relay is the multi-tenant heart of the server: it owns the live device
// WebSocket connections and turns each MCP tool call into a correlated
// request/response with the right device.
//
// This is the Go port of the v0 DeviceHub (server/src/device.ts), generalized
// from one device to many keyed by device_id.
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"abacad/internal/protocol"
)

// Errors surfaced to the MCP layer. The "no device connected" phrasing is load-
// bearing: smoke.mjs retries the first tool call while it still matches, to
// paper over the device connecting a beat after the agent.
var (
	ErrNoDevice   = errors.New("no device connected — open the Abacad app and connect it to this server")
	ErrDeviceGone = errors.New("device disconnected")
	ErrTimeout    = errors.New("device timed out")
)

// DefaultTimeout matches the v0 server's 15s per-command deadline.
const DefaultTimeout = 15 * time.Second

// DeviceConn is one live device WebSocket. All exported methods are safe for
// concurrent use: many MCP requests may target the same device at once.
type DeviceConn struct {
	DeviceID string

	ws      *websocket.Conn
	writeMu sync.Mutex // coder/websocket requires serialized writes
	seq     atomic.Uint64

	mu      sync.Mutex
	pending map[string]chan protocol.Reply

	closeOnce sync.Once
	closed    chan struct{}
}

// NewDeviceConn wraps an accepted WebSocket. The caller must run ReadPump.
func NewDeviceConn(deviceID string, ws *websocket.Conn) *DeviceConn {
	return &DeviceConn{
		DeviceID: deviceID,
		ws:       ws,
		pending:  make(map[string]chan protocol.Reply),
		closed:   make(chan struct{}),
	}
}

// Send issues a command and waits for the correlated reply. It returns the raw
// result JSON on success, or ErrTimeout / ErrDeviceGone / a device-reported
// error. timeout <= 0 uses DefaultTimeout.
func (c *DeviceConn) Send(ctx context.Context, method protocol.Method, params map[string]any, timeout time.Duration) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	select {
	case <-c.closed:
		return nil, ErrDeviceGone
	default:
	}

	id := strconv.FormatUint(c.seq.Add(1), 10)
	ch := make(chan protocol.Reply, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	raw, err := json.Marshal(protocol.Command{ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	err = c.ws.Write(ctx, websocket.MessageText, raw)
	c.writeMu.Unlock()
	if err != nil {
		return nil, ErrDeviceGone
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case reply := <-ch:
		if !reply.OK {
			msg := reply.Error
			if msg == "" {
				msg = "device reported an error"
			}
			return nil, errors.New(msg)
		}
		return reply.Result, nil
	case <-timer.C:
		return nil, ErrTimeout
	case <-c.closed:
		return nil, ErrDeviceGone
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReadPump reads replies until the socket closes, delivering each to the waiting
// Send by id. It blocks; run it in the connection's own goroutine. On return the
// connection is closed and all in-flight Sends have been failed.
func (c *DeviceConn) ReadPump(ctx context.Context) {
	defer c.close()
	for {
		typ, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var reply protocol.Reply
		if err := json.Unmarshal(data, &reply); err != nil {
			continue // non-JSON frame; ignore, matches v0
		}
		c.mu.Lock()
		ch, ok := c.pending[reply.ID]
		c.mu.Unlock()
		if !ok {
			continue // unknown/late id; drop, matches v0
		}
		ch <- reply // buffered(1); Send owns the receive
	}
}

// close is idempotent: it closes the socket, signals closed, and fails all
// pending waiters.
func (c *DeviceConn) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.ws != nil {
			_ = c.ws.Close(websocket.StatusNormalClosure, "bye")
		}
		// Pending waiters observe c.closed via their select; nothing else to do.
	})
}

// Close terminates the connection (used when the hub evicts a stale conn).
func (c *DeviceConn) Close() { c.close() }
