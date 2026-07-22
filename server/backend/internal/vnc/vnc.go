// Package vnc is the server side of the screen_recording live channel: a standard,
// decoupled VNC path. The device runs a normal VNC server bound to localhost and
// reverse-connects a dedicated WebSocket out to this server's ingress; a browser
// runs stock noVNC against the watch endpoint. This package is a byte-pipe between
// the two — it terminates both WebSockets and relays RFB frames verbatim. The
// pixels never touch the /device command socket or the /connect tunnel (that
// socket only carries the "start VNC / reverse-connect" trigger). See
// docs/screen-recording.md.
package vnc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"abacad/internal/protocol"
	"abacad/internal/relay"
	"abacad/internal/store"
)

const (
	// defaultTTL bounds a session's lifetime; a viewer link expires and the whole
	// session (device VNC included) is torn down after it, even if nobody watched.
	defaultTTL = 10 * time.Minute
	// startTimeout bounds the device's reply to the "start VNC" command.
	startTimeout = 15 * time.Second
	// readLimit caps a single relayed WebSocket message. RFB framebuffer updates
	// can be large; the device chunks them, but keep generous headroom.
	readLimit = 16 << 20
	// minStartInterval rate-limits session (re)starts per device, so a stuck or
	// hostile caller can't spam the device with start/stop VNC commands.
	minStartInterval = 2 * time.Second
)

// Account resolves the dashboard session on a /vnc/watch request to the viewing
// account. The viewer must be an authenticated human; the ticket alone is not
// enough. Returns an error to reject.
type Account func(r *http.Request) (store.Account, error)

// Manager owns live VNC sessions, one per device. It mints the device-side ingress
// token and the browser-side viewer ticket, tells the device (over the command
// hub) to start its VNC server and reverse-connect, and bridges the two sockets.
type Manager struct {
	hub         *relay.Hub
	ingressBase string  // e.g. "wss://abacad.ai" — device reverse-connects to ingressBase + "/vnc/ingress?token=…"
	account     Account // dashboard-session auth for the browser watch side

	// audit records a session-boundary event (opened / viewer-connected / closed)
	// for the activity trail; set by the host. nil disables it. The device-side
	// "vnc" command is already logged by the device handler's command observer;
	// this covers the browser side, which never touches that path.
	audit func(accountID, deviceID, event string)

	mu        sync.Mutex
	byDevice  map[string]*session
	byIngress map[string]*session
	byTicket  map[string]*session
	lastStart map[string]time.Time // per-device cooldown to rate-limit start
}

// NewManager builds a Manager. ingressBase is the wss origin the device dials back
// to (no trailing slash); account authorizes browser viewers.
func NewManager(hub *relay.Hub, ingressBase string, account Account) *Manager {
	return &Manager{
		hub: hub, ingressBase: ingressBase, account: account,
		byDevice:  map[string]*session{},
		byIngress: map[string]*session{},
		byTicket:  map[string]*session{},
		lastStart: map[string]time.Time{},
	}
}

// SetAudit installs the session-boundary audit hook.
func (m *Manager) SetAudit(f func(accountID, deviceID, event string)) { m.audit = f }

func (m *Manager) recordAudit(accountID, deviceID, event string) {
	if m.audit != nil {
		m.audit(accountID, deviceID, event)
	}
}

// Session is the public view of a live session's state.
type Session struct {
	Active          bool      `json:"active"`
	ViewerConnected bool      `json:"viewer_connected"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type session struct {
	mgr        *Manager
	deviceID   string
	accountID  string
	ingressTok string
	ticket     string
	expiresAt  time.Time

	mu        sync.Mutex
	device    *websocket.Conn
	viewer    *websocket.Conn
	linked    bool
	closed    bool
	done      chan struct{}
	closeOnce sync.Once
}

// Start opens (or restarts) the live session for a device the caller owns. It
// mints the tokens, registers the session, and tells the device to start its VNC
// server and reverse-connect to the ingress. Returns the viewer ticket + expiry;
// the caller builds the noVNC URL as "/vnc/watch?ticket=<ticket>".
func (m *Manager) Start(ctx context.Context, deviceID, accountID string) (ticket string, expiresAt time.Time, err error) {
	dc, ok := m.hub.Get(deviceID)
	if !ok {
		return "", time.Time{}, relay.ErrNoDevice
	}

	// Rate-limit (re)starts per device.
	m.mu.Lock()
	if last, ok := m.lastStart[deviceID]; ok && time.Since(last) < minStartInterval {
		m.mu.Unlock()
		return "", time.Time{}, errors.New("live view was just started for this device — wait a moment and retry")
	}
	m.lastStart[deviceID] = time.Now()
	m.mu.Unlock()

	m.Stop(deviceID) // one session per device: replace any existing

	s := &session{
		mgr: m, deviceID: deviceID, accountID: accountID,
		ingressTok: randToken(), ticket: randToken(),
		expiresAt: time.Now().Add(defaultTTL),
		done:      make(chan struct{}),
	}
	m.mu.Lock()
	m.byDevice[deviceID] = s
	m.byIngress[s.ingressTok] = s
	m.byTicket[s.ticket] = s
	m.mu.Unlock()

	// Reverse-connect the device to the SAME origin it dialed (dev or prod),
	// falling back to the configured base only if the origin is unknown.
	base := dc.Origin()
	if base == "" {
		base = m.ingressBase
	}
	url := base + "/vnc/ingress?token=" + s.ingressTok
	if _, err := dc.Send(ctx, protocol.MethodVNC,
		map[string]any{"action": "start", "url": url, "token": s.ingressTok}, startTimeout); err != nil {
		m.remove(s)
		return "", time.Time{}, err
	}

	m.recordAudit(accountID, deviceID, "live_view_started")
	time.AfterFunc(defaultTTL, s.close)
	return s.ticket, s.expiresAt, nil
}

// Stop ends any live session for a device (dashboard "stop", kill, or replacement).
func (m *Manager) Stop(deviceID string) {
	m.mu.Lock()
	s := m.byDevice[deviceID]
	m.mu.Unlock()
	if s != nil {
		s.close()
	}
}

// Status reports a device's live session for the dashboard / MCP status.
func (m *Manager) Status(deviceID string) Session {
	m.mu.Lock()
	s := m.byDevice[deviceID]
	m.mu.Unlock()
	if s == nil {
		return Session{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Session{Active: !s.closed, ViewerConnected: s.viewer != nil, ExpiresAt: s.expiresAt}
}

// ServeIngress accepts the device's reverse VNC WebSocket, matched by its ingress
// token. It blocks until the session ends.
func (m *Manager) ServeIngress(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	s := m.byIngress[r.URL.Query().Get("token")]
	m.mu.Unlock()
	if s == nil {
		http.Error(w, "unknown vnc session", http.StatusNotFound)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	c.SetReadLimit(readLimit)
	s.attach(c, true)
}

// ServeWatch accepts the browser's noVNC WebSocket. The viewer must be an
// authenticated human whose account owns the session, AND present the ticket.
func (m *Manager) ServeWatch(w http.ResponseWriter, r *http.Request) {
	acc, err := m.account(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Single-use ticket: consume it on first use so a leaked ticket can't be
	// replayed. The account cookie still gates every connection either way.
	m.mu.Lock()
	tkt := r.URL.Query().Get("ticket")
	s := m.byTicket[tkt]
	if s != nil {
		delete(m.byTicket, tkt)
	}
	m.mu.Unlock()
	if s == nil || s.accountID != acc.ID {
		http.Error(w, "unknown or already-used vnc ticket", http.StatusNotFound)
		return
	}
	// noVNC historically offers the "binary" subprotocol; select it if offered.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		Subprotocols:       []string{"binary"},
	})
	if err != nil {
		return
	}
	c.SetReadLimit(readLimit)
	m.recordAudit(s.accountID, s.deviceID, "live_view_viewer_connected")
	s.attach(c, false)
}

// attach installs one side of the bridge and blocks the HTTP handler until the
// session tears down. When both sides are present the pipe starts.
func (s *session) attach(c *websocket.Conn, isDevice bool) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = c.Close(websocket.StatusGoingAway, "vnc session closed")
		return
	}
	if isDevice {
		s.device = c
	} else if s.viewer != nil {
		// Single viewer per session; reject a second.
		s.mu.Unlock()
		_ = c.Close(websocket.StatusPolicyViolation, "vnc session already has a viewer")
		return
	} else {
		s.viewer = c
	}
	if !s.linked && s.device != nil && s.viewer != nil {
		s.linked = true
		go s.pipe(s.device, s.viewer)
	}
	s.mu.Unlock()
	<-s.done
}

// pipe relays RFB frames both ways until either side closes, then ends the session.
func (s *session) pipe(device, viewer *websocket.Conn) {
	ctx := context.Background()
	go func() { copyWS(ctx, device, viewer); s.close() }() // device -> viewer
	copyWS(ctx, viewer, device)                            // viewer -> device
	s.close()
}

func copyWS(ctx context.Context, src, dst *websocket.Conn) {
	for {
		typ, data, err := src.Read(ctx)
		if err != nil {
			return
		}
		if err := dst.Write(ctx, typ, data); err != nil {
			return
		}
	}
}

// close tears the session down once: closes both sockets, unregisters it, tells
// the device to stop its VNC server, and unblocks the attached handlers.
func (s *session) close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		d, v := s.device, s.viewer
		s.mu.Unlock()
		if d != nil {
			_ = d.Close(websocket.StatusNormalClosure, "vnc ended")
		}
		if v != nil {
			_ = v.Close(websocket.StatusNormalClosure, "vnc ended")
		}
		s.mgr.remove(s)
		s.mgr.tellDeviceStop(s.deviceID)
		s.mgr.recordAudit(s.accountID, s.deviceID, "live_view_ended")
		close(s.done)
	})
}

func (m *Manager) remove(s *session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byDevice[s.deviceID] == s {
		delete(m.byDevice, s.deviceID)
	}
	delete(m.byIngress, s.ingressTok)
	delete(m.byTicket, s.ticket)
}

// tellDeviceStop best-effort asks the device to stop its VNC server; runs
// detached so teardown never blocks on a slow/gone device.
func (m *Manager) tellDeviceStop(deviceID string) {
	if m.hub == nil {
		return
	}
	dc, ok := m.hub.Get(deviceID)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = dc.Send(ctx, protocol.MethodVNC, map[string]any{"action": "stop"}, 5*time.Second)
	}()
}

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
