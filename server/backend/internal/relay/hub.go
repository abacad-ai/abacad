package relay

import (
	"sync"

	"abacad/internal/protocol"
)

// Hub maps device_id -> the single live DeviceConn for that device. A device may
// reconnect (network blip, app restart); a new connection for an id evicts the
// old one so the id always points at the freshest socket.
//
// Account isolation is NOT enforced here — the hub only knows device ids. The
// MCP layer resolves an authenticated account to the set of device ids it owns
// before ever touching the hub.
type Hub struct {
	mu    sync.Mutex
	conns map[string]*DeviceConn
	gate  Gate
}

// NewHub creates an empty hub. gate authorizes every command and stream on every
// connection the hub registers. It is a required argument rather than a settable
// field so that "no gate" cannot arise by omission; pass AllowAllGate to opt out
// explicitly and visibly.
func NewHub(gate Gate) *Hub {
	if gate == nil {
		panic("relay.NewHub: nil gate — pass relay.AllowAllGate to allow everything explicitly")
	}
	return &Hub{conns: make(map[string]*DeviceConn), gate: gate}
}

// Register installs dc as the live connection for its device id, evicting and
// closing any previous connection for the same id. It also injects the hub's
// capability gate — which is why a connection is only drivable once registered.
func (h *Hub) Register(dc *DeviceConn) {
	h.mu.Lock()
	dc.gate = h.gate
	old := h.conns[dc.DeviceID]
	h.conns[dc.DeviceID] = dc
	h.mu.Unlock()
	if old != nil && old != dc {
		old.Close() // its ReadPump returns and calls Remove(old), a no-op below
	}
}

// Remove drops dc from the hub, but only if the hub still points at this exact
// connection — so a just-evicted stale conn's cleanup can't unregister the fresh
// one that replaced it.
func (h *Hub) Remove(dc *DeviceConn) {
	h.mu.Lock()
	if h.conns[dc.DeviceID] == dc {
		delete(h.conns, dc.DeviceID)
	}
	h.mu.Unlock()
}

// Kick force-closes a device's live connection, if any, and reports whether one
// was closed. Used by the enrollment-expiry sweeper: an in-flight socket bypasses
// the connect-time token check, so expiry has to actively drop it; the lookup
// filter then blocks any reconnect. The closed conn's ReadPump calls Remove.
func (h *Hub) Kick(deviceID string) bool {
	dc, ok := h.Get(deviceID)
	if ok {
		dc.Close()
	}
	return ok
}

// Get returns the live connection for a device id, if any.
func (h *Hub) Get(deviceID string) (*DeviceConn, bool) {
	h.mu.Lock()
	dc, ok := h.conns[deviceID]
	h.mu.Unlock()
	return dc, ok
}

// Online reports whether a device id currently has a live connection.
func (h *Hub) Online(deviceID string) bool {
	_, ok := h.Get(deviceID)
	return ok
}

// Activity returns a device's last-reported power state and whether it's online.
// Offline devices report ("", false); the caller shows no activity for them.
func (h *Hub) Activity(deviceID string) (protocol.Activity, bool) {
	dc, ok := h.Get(deviceID)
	if !ok {
		return "", false
	}
	return dc.Activity(), true
}

// OnlineIDs returns the set of device ids with a live connection.
func (h *Hub) OnlineIDs() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make(map[string]bool, len(h.conns))
	for id := range h.conns {
		ids[id] = true
	}
	return ids
}
