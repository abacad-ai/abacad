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
	"fmt"
	"io"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"abacad/internal/protocol"
)

// CommandRecord is the outcome of one device command, handed to a CommandObserver
// for the activity log. It mirrors what gets logged.
type CommandRecord struct {
	DeviceID string
	Method   string
	Source   string // agent | dashboard
	Duration time.Duration
	Outcome  string // ok | timeout | device_gone | canceled | error
	Detail   string // error message when Outcome == error
}

// CommandObserver is notified when a device command completes. It runs inline on
// the caller's goroutine, so it must be cheap and non-blocking. nil disables it.
type CommandObserver func(CommandRecord)

// sourceKey tags a request context with who is driving (agent vs dashboard), so
// the activity log can tell an agent's tap from the dashboard's screenshot poll.
type sourceKey struct{}

// WithSource returns a context that labels commands issued under it. Empty src is
// ignored (Send defaults to "agent").
func WithSource(ctx context.Context, src string) context.Context {
	return context.WithValue(ctx, sourceKey{}, src)
}

func sourceFrom(ctx context.Context) string {
	if s, ok := ctx.Value(sourceKey{}).(string); ok && s != "" {
		return s
	}
	return "agent"
}

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

	streamSeq atomic.Uint64
	streamsMu sync.Mutex
	streams   map[uint64]*Stream

	onCmd CommandObserver // may be nil; notified on every Send completion

	reasonMu    sync.Mutex
	closeReason string // why ReadPump exited; read after the pump returns

	closeOnce sync.Once
	closed    chan struct{}
}

// NewDeviceConn wraps an accepted WebSocket. The caller must run ReadPump.
func NewDeviceConn(deviceID string, ws *websocket.Conn) *DeviceConn {
	return &DeviceConn{
		DeviceID: deviceID,
		ws:       ws,
		pending:  make(map[string]chan protocol.Reply),
		streams:  make(map[uint64]*Stream),
		closed:   make(chan struct{}),
	}
}

// SetCommandObserver installs (or clears) the per-command observer. Call before
// ReadPump starts.
func (c *DeviceConn) SetCommandObserver(obs CommandObserver) { c.onCmd = obs }

// writeFrame serializes one WebSocket write. coder/websocket requires writes be
// serialized, and commands (text) and tunnel frames (binary) share the socket,
// so both go through here.
func (c *DeviceConn) writeFrame(ctx context.Context, typ websocket.MessageType, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.Write(ctx, typ, data)
}

// OpenStream asks the device to dial target ("host:port") and returns a Stream
// bridging to it. The dial is optimistic: OpenStream returns as soon as the OPEN
// frame is sent, and a dial failure surfaces as an error on the first Read.
func (c *DeviceConn) OpenStream(ctx context.Context, target string) (*Stream, error) {
	select {
	case <-c.closed:
		return nil, ErrDeviceGone
	default:
	}
	id := c.streamSeq.Add(1)
	s := &Stream{
		id:       id,
		conn:     c,
		in:       make(chan []byte, streamBufferFrames),
		closed:   make(chan struct{}),
		closeErr: io.EOF,
	}
	c.streamsMu.Lock()
	c.streams[id] = s
	c.streamsMu.Unlock()

	frame := protocol.EncodeStreamFrame(protocol.StreamOpen, id, []byte(target))
	if err := c.writeFrame(ctx, websocket.MessageBinary, frame); err != nil {
		c.removeStream(id)
		return nil, ErrDeviceGone
	}
	return s, nil
}

func (c *DeviceConn) removeStream(id uint64) {
	c.streamsMu.Lock()
	delete(c.streams, id)
	c.streamsMu.Unlock()
}

// handleStreamFrame routes an inbound binary frame to its stream. Unknown ids
// (already closed, or never opened) are dropped, matching how late command
// replies are dropped.
func (c *DeviceConn) handleStreamFrame(buf []byte) {
	t, id, payload, err := protocol.DecodeStreamFrame(buf)
	if err != nil {
		return
	}
	c.streamsMu.Lock()
	s := c.streams[id]
	c.streamsMu.Unlock()
	if s == nil {
		return
	}
	switch t {
	case protocol.StreamData:
		b := make([]byte, len(payload)) // payload aliases the read buffer; copy to retain
		copy(b, payload)
		s.deliver(b)
	case protocol.StreamClose:
		s.finish(closeCause(payload), false)
	case protocol.StreamOpen:
		// Devices never open streams; ignore.
	}
}

// Send issues a command and waits for the correlated reply. It returns the raw
// result JSON on success, or ErrTimeout / ErrDeviceGone / a device-reported
// error. timeout <= 0 uses DefaultTimeout.
//
// Every call is logged and (if an observer is set) recorded — this is the single
// choke point that makes a hung or failed command visible instead of silent.
func (c *DeviceConn) Send(ctx context.Context, method protocol.Method, params map[string]any, timeout time.Duration) (result json.RawMessage, err error) {
	start := time.Now()
	defer func() {
		dur := time.Since(start)
		outcome, detail := classify(err)
		src := sourceFrom(ctx)
		if detail != "" {
			log.Printf("[cmd] device=%s src=%s method=%s dur=%dms result=%s: %s",
				c.DeviceID, src, method, dur.Milliseconds(), outcome, detail)
		} else {
			log.Printf("[cmd] device=%s src=%s method=%s dur=%dms result=%s",
				c.DeviceID, src, method, dur.Milliseconds(), outcome)
		}
		if c.onCmd != nil {
			c.onCmd(CommandRecord{
				DeviceID: c.DeviceID, Method: string(method), Source: src,
				Duration: dur, Outcome: outcome, Detail: detail,
			})
		}
	}()

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

// classify maps a Send error to an activity-log outcome + optional detail. The
// sentinels get clean labels; anything else is a device-reported error whose
// message is worth keeping.
func classify(err error) (outcome, detail string) {
	switch {
	case err == nil:
		return "ok", ""
	case errors.Is(err, ErrTimeout):
		return "timeout", ""
	case errors.Is(err, ErrDeviceGone):
		return "device_gone", ""
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled", err.Error()
	default:
		return "error", err.Error()
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
			c.setCloseReason(err)
			return
		}
		if typ == websocket.MessageBinary {
			c.handleStreamFrame(data) // tunnel lane
			continue
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
// pending waiters and live streams.
func (c *DeviceConn) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.ws != nil {
			_ = c.ws.Close(websocket.StatusNormalClosure, "bye")
		}
		// Pending command waiters observe c.closed via their select. Streams have
		// their own close signal, so tear each down explicitly.
		c.streamsMu.Lock()
		live := make([]*Stream, 0, len(c.streams))
		for _, s := range c.streams {
			live = append(live, s)
		}
		c.streamsMu.Unlock()
		for _, s := range live {
			s.finish(ErrDeviceGone, false)
		}
	})
}

// Close terminates the connection (used when the hub evicts a stale conn).
func (c *DeviceConn) Close() { c.close() }

// setCloseReason records why ReadPump exited, translating a clean WebSocket close
// into "close <code> <reason>" and leaving raw I/O errors (network drop, read
// limit) as-is.
func (c *DeviceConn) setCloseReason(err error) {
	reason := err.Error()
	var ce websocket.CloseError
	if errors.As(err, &ce) {
		if ce.Reason != "" {
			reason = fmt.Sprintf("close %d (%s)", ce.Code, ce.Reason)
		} else {
			reason = fmt.Sprintf("close %d", ce.Code)
		}
	}
	c.reasonMu.Lock()
	c.closeReason = reason
	c.reasonMu.Unlock()
}

// CloseReason returns why the connection dropped, once ReadPump has returned. It
// reads "connection closed" if nothing more specific was captured (e.g. an
// eviction closed the socket from our side).
func (c *DeviceConn) CloseReason() string {
	c.reasonMu.Lock()
	defer c.reasonMu.Unlock()
	if c.closeReason == "" {
		return "connection closed"
	}
	return c.closeReason
}
