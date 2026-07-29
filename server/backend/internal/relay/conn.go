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
	Actor    Actor  // which credential issued it, and from where
}

// Actor identifies the caller behind a command for the activity trail: which
// credential, and the address it came from. It rides the request context because
// the record is emitted deep in Send, far from any http.Request — and, for
// relayed commands, on a goroutine servicing the *device's* socket rather than
// the caller's.
//
// Label is a snapshot of the credential's name at call time, not a live lookup,
// so revoking or renaming an API key leaves its past actions attributable.
type Actor struct {
	Kind      string // store.ActorSession | ActorAPIKey | ActorDevice | ActorSSH
	ID        string // apikey_<random>, session id, device id, ssh key id
	Label     string // human-readable name as of now
	IP        string // client address (see api.clientIP)
	UserAgent string
}

// CommandObserver is notified when a device command completes. It runs inline on
// the caller's goroutine, so it must be cheap and non-blocking. nil disables it.
type CommandObserver func(CommandRecord)

// sourceKey tags a request context with who is driving (agent vs dashboard), so
// the activity log can tell an agent's tap from the dashboard's screenshot poll.
type sourceKey struct{}

// actorKey tags a request context with the credential driving it. Separate from
// sourceKey on purpose: source is the channel (agent | dashboard | ssh | tunnel),
// actor is the identity, and the two vary independently — one API key can drive
// both an /mcp call and a /connect tunnel.
type actorKey struct{}

// WithSource returns a context that labels commands issued under it. Empty src is
// ignored (Send defaults to "agent").
func WithSource(ctx context.Context, src string) context.Context {
	return context.WithValue(ctx, sourceKey{}, src)
}

// WithActor returns a context carrying the credential behind the commands issued
// under it. Commands sent without one still record — just with a blank actor —
// so an un-stamped call path degrades to today's behaviour rather than failing.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom returns the actor stamped on ctx, or the zero Actor if none.
func ActorFrom(ctx context.Context) Actor {
	a, _ := ctx.Value(actorKey{}).(Actor)
	return a
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
	ErrNoDevice   = errors.New("no device connected — open the abacad app and connect it to this server")
	ErrDeviceGone = errors.New("device disconnected")
	ErrTimeout    = errors.New("device timed out")
)

// DefaultTimeout matches the v0 server's 15s per-command deadline.
const DefaultTimeout = 15 * time.Second

// Server-side liveness. The client already pings every 20s, but a half-open
// socket (the phone froze in Doze, the radio dropped, a NAT rebinding) leaves
// the server's Read blocked with no error — so without our own probe a gone
// device lingers as "online" (worse, as "asleep") until TCP eventually breaks.
// We ping the device on an interval and require a pong within a deadline; enough
// consecutive misses closes the socket, which drops it from the hub → honestly
// offline. This is what makes "asleep" (still answering) mean something
// different from "offline" (not answering). The interval sits above the client's
// 20s so the two don't beat against each other.
//
// One miss is NOT enough. A real device freezes for reasons that aren't death:
// a Mac naps, an Android Doze window suspends the radio, a handover moves the
// socket, or a multi-MB screenshot write holds the library's write lock past the
// probe's deadline. Dropping on the first miss made every one of those a
// disconnect + reconnect, so an idle device churned all night. We ride out
// pongMissBudget probes before giving up; the cost is that "offline" can lag
// reality by ~100s.
//
// The write-lock case is only covered while the ping is waiting to ACQUIRE the
// lock. If it gets the lock and the write itself stalls (the peer's receive
// window is shut), coder/websocket's own 5s write timeout force-closes the
// connection out from under us — no miss budget can ride that out.
const (
	pingInterval   = 30 * time.Second
	pongTimeout    = 10 * time.Second
	pongMissBudget = 3
)

// DeviceConn is one live device WebSocket. All exported methods are safe for
// concurrent use: many MCP requests may target the same device at once.
type DeviceConn struct {
	DeviceID string

	// origin is the ws(s) origin the device dialed to reach /device, e.g.
	// "wss://abacad.ai" or "ws://localhost:1213" — derived from its connect
	// request. The VNC live channel uses it so the device reverse-connects to the
	// SAME server (and scheme) it's already on, in dev and prod alike, instead of a
	// hardcoded domain. Set once before Register, read later; safe without a lock.
	origin string

	ws      *websocket.Conn
	writeMu sync.Mutex // coder/websocket requires serialized writes
	seq     atomic.Uint64

	mu      sync.Mutex
	pending map[string]chan protocol.Reply

	streamSeq atomic.Uint64
	streamsMu sync.Mutex
	streams   map[uint64]*Stream

	onCmd CommandObserver // may be nil; notified on every Send completion

	// gate authorizes each command and stream against the device's configured
	// capabilities. Injected by Hub.Register, so a connection that was never
	// registered has none — and no gate denies (see DeviceConn.capability).
	// Deliberately a function, not cached state: see the Gate doc comment.
	gate Gate

	// humanize mirrors the device record's humanize setting, refreshed on each
	// Resolve. Read when building pointer commands so the client knows whether to
	// synthesize human-like motion. Defaults OFF; opt-in per device.
	humanize atomic.Bool

	// activity holds the device's last-reported power state (protocol.Activity).
	// Defaults to active; updated by presence frames in ReadPump. It's a display
	// signal only — the device stays reachable while asleep, so it doesn't gate
	// command routing.
	activity atomic.Value

	// Liveness probe cadence; defaulted from pingInterval/pongTimeout/
	// pongMissBudget in NewDeviceConn. Fields (not the consts directly) so tests
	// can shrink them.
	pingInterval   time.Duration
	pongTimeout    time.Duration
	pongMissBudget int

	// missedPongs counts every unanswered probe over the connection's life
	// (not just the consecutive run that ends it). Lets a test prove it actually
	// exercised a miss rather than passing on timing luck.
	missedPongs atomic.Int64

	reasonMu    sync.Mutex
	closeReason string // why ReadPump exited; read after the pump returns

	closeOnce sync.Once
	closed    chan struct{}
}

// SetOrigin records the ws(s) origin the device dialed (see the origin field).
// Call before Register.
func (c *DeviceConn) SetOrigin(o string) { c.origin = o }

// Origin returns the ws(s) origin the device dialed, or "" if unset.
func (c *DeviceConn) Origin() string { return c.origin }

// NewDeviceConn wraps an accepted WebSocket. The caller must run ReadPump.
func NewDeviceConn(deviceID string, ws *websocket.Conn) *DeviceConn {
	c := &DeviceConn{
		DeviceID: deviceID,
		ws:       ws,
		pending:  make(map[string]chan protocol.Reply),
		streams:  make(map[uint64]*Stream),
		closed:   make(chan struct{}),
	}
	c.activity.Store(protocol.ActivityActive) // assume awake until told otherwise
	c.humanize.Store(false)                   // off unless the device record opts in
	c.pingInterval = pingInterval
	c.pongTimeout = pongTimeout
	c.pongMissBudget = pongMissBudget
	return c
}

// SetCommandObserver installs (or clears) the per-command observer. Call before
// ReadPump starts.
func (c *DeviceConn) SetCommandObserver(obs CommandObserver) { c.onCmd = obs }

// SetHumanize records whether this device wants human-like pointer motion,
// mirroring the store record. Refreshed on every Resolve.
func (c *DeviceConn) SetHumanize(v bool) { c.humanize.Store(v) }

// Humanize reports the device's current humanize setting.
func (c *DeviceConn) Humanize() bool { return c.humanize.Load() }

// Activity returns the device's last-reported power state. A fresh connection is
// active until a presence frame says otherwise.
func (c *DeviceConn) Activity() protocol.Activity {
	if a, ok := c.activity.Load().(protocol.Activity); ok {
		return a
	}
	return protocol.ActivityActive
}

// setActivity records a device-reported power state, ignoring unknown values. A
// real transition is logged so the server trail shows when a device slept/woke.
func (c *DeviceConn) setActivity(a protocol.Activity) {
	if a != protocol.ActivityActive && a != protocol.ActivityAsleep {
		return
	}
	prev, _ := c.activity.Load().(protocol.Activity)
	if a == prev {
		return
	}
	c.activity.Store(a)
	log.Printf("[device] device=%s activity=%s", c.DeviceID, a)
}

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
//
// cap names which capability this stream is opened under — CapTunnel for a
// /connect tunnel, CapSSH for the jump host. Callers must pass it explicitly so
// that a future third consumer of the tunnel lane has to declare itself rather
// than silently inheriting someone else's authorization.
func (c *DeviceConn) OpenStream(ctx context.Context, target string, need protocol.Capability) (*Stream, error) {
	if err := c.capability(need); err != nil {
		return nil, err
	}
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
		actor := ActorFrom(ctx)
		// actor=<label or id> is appended only when stamped, so log lines for
		// un-attributed paths keep their existing shape.
		who := actor.Label
		if who == "" {
			who = actor.ID
		}
		suffix := ""
		if who != "" {
			suffix = fmt.Sprintf(" actor=%s", who)
		}
		if actor.IP != "" {
			suffix += " ip=" + actor.IP
		}
		if detail != "" {
			log.Printf("[cmd] device=%s src=%s%s method=%s dur=%dms result=%s: %s",
				c.DeviceID, src, suffix, method, dur.Milliseconds(), outcome, detail)
		} else {
			log.Printf("[cmd] device=%s src=%s%s method=%s dur=%dms result=%s",
				c.DeviceID, src, suffix, method, dur.Milliseconds(), outcome)
		}
		if c.onCmd != nil {
			c.onCmd(CommandRecord{
				DeviceID: c.DeviceID, Method: string(method), Source: src,
				Duration: dur, Outcome: outcome, Detail: detail, Actor: actor,
			})
		}
	}()

	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	// Authorize before anything else, so the deferred observer above records the
	// denial on the activity trail. A denial the owner cannot see is how a
	// compromised agent maps the capability matrix unnoticed.
	if err = c.authorizeMethod(method, params); err != nil {
		return nil, err
	}
	if method == protocol.MethodComposite {
		// composite carries its own step list, each step an action in its own
		// right. Authorizing only the outer verb would let one permitted call
		// smuggle every denied one.
		if params, err = c.authorizeComposite(params); err != nil {
			return nil, err
		}
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
	case errors.Is(err, ErrCapabilityDenied):
		// Its own outcome, not a generic error: a denial is the gate working as
		// configured, and the owner needs to see it on the trail. Without this,
		// probing which capabilities a device exposes leaves no trace.
		return "denied", err.Error()
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
	go c.pingLoop(ctx) // probes liveness; exits when the socket closes
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
		// Presence frames are unsolicited (no id) and tagged; handle them before
		// the reply path so they don't get dropped as an unknown id.
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &probe) == nil && probe.Type == "presence" {
			var p protocol.Presence
			if json.Unmarshal(data, &p) == nil {
				c.setActivity(p.State)
			}
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

// pingLoop probes the device on an interval and requires a pong within
// pongTimeout. A single miss is tolerated (see the pongMissBudget comment); only
// pongMissBudget *consecutive* misses record the reason and close the socket,
// which unblocks ReadPump and drops the device from the hub. Any answered ping
// resets the count, so a device that skips one probe and comes back stays
// connected. It exits when the socket closes for any reason.
//
// coder/websocket requires Ping run concurrently with the reader (ReadPump),
// which it always is, and Ping serializes its own control-frame write, so it's
// safe alongside Send. Ping does not close the connection when its context
// expires — it just returns the error — which is what lets us ride out a miss.
func (c *DeviceConn) pingLoop(ctx context.Context) {
	t := time.NewTicker(c.pingInterval)
	defer t.Stop()
	misses := 0
	var firstMiss time.Time
	for {
		select {
		case <-c.closed:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, c.pongTimeout)
			err := c.ws.Ping(pctx)
			cancel()
			if err == nil {
				if misses > 0 {
					log.Printf("[device] device=%s pong resumed after %d missed probe(s)", c.DeviceID, misses)
				}
				misses = 0
				continue
			}
			select {
			case <-c.closed:
				return // already closing; the real reason is set elsewhere
			default:
			}
			misses++
			c.missedPongs.Add(1)
			if misses == 1 {
				firstMiss = time.Now()
			}
			if misses < c.pongMissBudget {
				log.Printf("[device] device=%s missed pong (%d/%d)", c.DeviceID, misses, c.pongMissBudget)
				continue
			}
			c.setCloseReason(fmt.Errorf("no pong across %d probes over %s",
				misses, time.Since(firstMiss).Round(time.Second)))
			// The peer is by definition not answering, so don't spend the close
			// handshake's 5s waiting for its close frame — that delay lands in the
			// disconnect's logged uptime and helps nobody.
			c.closeWith(true)
			return
		}
	}
}

// close is idempotent: it closes the socket, signals closed, and fails all
// pending waiters and live streams.
func (c *DeviceConn) close() { c.closeWith(false) }

// closeWith is close with a choice of goodbye. now=false sends a close frame and
// waits (up to 5s, inside coder/websocket) for the peer's reply — the polite
// path, used for eviction and operator kicks, where the client is listening and
// should see a clean 1000. now=true tears the socket down immediately, for a
// peer that has stopped answering and would only make us wait out that timeout.
func (c *DeviceConn) closeWith(now bool) {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.ws != nil {
			if now {
				_ = c.ws.CloseNow()
			} else {
				_ = c.ws.Close(websocket.StatusNormalClosure, "bye")
			}
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
// limit) as-is. First writer wins: when the ping loop closes the socket it sets
// the true reason ("no pong…") first, and the "use of closed connection" error
// ReadPump then observes must not clobber it.
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
	if c.closeReason == "" {
		c.closeReason = reason
	}
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
