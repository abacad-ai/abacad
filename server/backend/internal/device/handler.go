// Package device serves the /device WebSocket that a device dials into and holds
// open. It authenticates the device (Phase 3+: a per-device token in the query),
// registers it with the relay hub under its device id, and pumps replies until
// the socket drops.
package device

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"abacad/internal/events"
	"abacad/internal/relay"
)

// readLimit lifts coder/websocket's 32 KiB default, which would otherwise
// silently reject screenshot payloads (base64 JPEGs run to a few MB).
//
// It is a per-MESSAGE receive cap, not a transfer cap: the reader buffers a whole
// message in memory before handing it up, so this bounds that allocation. We keep
// it small on purpose — big transfers (files) are chunked into small messages, so
// nothing legitimate approaches this. It also lines up with the device's okhttp
// client, whose fixed 16 MiB *outbound* queue (RealWebSocket.MAX_QUEUE_SIZE, not
// configurable) already caps a single device->server frame at 16 MiB regardless.
const readLimit = 16 << 20 // 16 MiB

// Resolver maps an inbound /device request to the device id it may register as.
// Phase 1 uses a fixed id; Phase 3 swaps in a token lookup. Returning an error
// rejects the connection.
type Resolver func(r *http.Request) (deviceID string, err error)

// Seen is an optional hook fired when a device connects or replies, so the store
// can update last_seen. nil is fine.
type Seen func(deviceID string)

// Handler builds the /device HTTP handler.
type Handler struct {
	Hub     *relay.Hub
	Resolve Resolver
	OnSeen  Seen
	Events  *events.Log // per-device activity log; may be nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deviceID, err := h.Resolve(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // non-browser client (Android app / mock); no Origin to check
	})
	if err != nil {
		return // Accept already wrote the error
	}
	c.SetReadLimit(readLimit)

	if h.OnSeen != nil {
		h.OnSeen(deviceID)
	}
	log.Printf("[device] connected: %s from %s", deviceID, r.RemoteAddr)

	dc := relay.NewDeviceConn(deviceID, c)
	if h.Events != nil {
		dc.SetCommandObserver(func(rec relay.CommandRecord) {
			h.Events.Append(rec.DeviceID, events.Event{
				Kind:       events.KindCommand,
				Method:     rec.Method,
				Source:     rec.Source,
				DurationMs: rec.Duration.Milliseconds(),
				Outcome:    rec.Outcome,
				Detail:     rec.Detail,
			})
		})
		h.Events.Append(deviceID, events.Event{Kind: events.KindConnected})
	}
	h.Hub.Register(dc)

	// ReadPump blocks until the socket closes.
	start := time.Now()
	dc.ReadPump(context.Background())
	h.Hub.Remove(dc)

	reason := dc.CloseReason()
	uptime := time.Since(start).Round(time.Second)
	log.Printf("[device] disconnected: %s reason=%q after=%s", deviceID, reason, uptime)
	if h.Events != nil {
		h.Events.Append(deviceID, events.Event{Kind: events.KindDisconnected, Detail: reason})
	}
}
