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
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{conns: make(map[string]*DeviceConn)}
}

// Register installs dc as the live connection for its device id, evicting and
// closing any previous connection for the same id.
func (h *Hub) Register(dc *DeviceConn) {
	h.mu.Lock()
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
