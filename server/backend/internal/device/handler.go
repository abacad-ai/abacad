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

	"abacad/internal/activity"
	"abacad/internal/events"
	"abacad/internal/relay"
	"abacad/internal/store"
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

// Resolver maps an inbound /device request to the device id it may register as
// and the account that owns it (attributed in the activity trail). Returning an
// error rejects the connection.
type Resolver func(r *http.Request) (deviceID, accountID string, err error)

// Seen is an optional hook fired when a device connects or replies, so the store
// can update last_seen. nil is fine.
type Seen func(deviceID string)

// Reported is an optional hook fired once on connect with the version the client
// advertised in the dial (?version=<v>), so the store can record it. Blank when
// the client doesn't report one; a nil hook is fine.
type Reported func(deviceID, version string)

// Handler builds the /device HTTP handler.
type Handler struct {
	Hub       *relay.Hub
	Resolve   Resolver
	OnSeen    Seen
	OnVersion Reported           // records the client-reported version; may be nil
	Events    *events.Log        // per-device live ring; may be nil
	Activity  *activity.Recorder // persistent account trail; may be nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deviceID, accountID, err := h.Resolve(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Skip the same-origin check on purpose. Native clients (Android/mac) send
		// no Origin, and a browser device client is served from its own page and
		// dials in with an Origin that need not match this host (e.g. an
		// <id>.abacad.ai surface hitting the API host). Origin is not the security
		// boundary here: the connection is authenticated by the per-device token in
		// the query string, not by an ambient cookie, so cross-site forgery can't
		// mint a valid device connection.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return // Accept already wrote the error
	}
	c.SetReadLimit(readLimit)

	if h.OnSeen != nil {
		h.OnSeen(deviceID)
	}
	reportedVersion := r.URL.Query().Get("version")
	if h.OnVersion != nil {
		h.OnVersion(deviceID, reportedVersion)
	}
	log.Printf("[device] connected: %s from %s (client %s)", deviceID, r.RemoteAddr, versionLabel(reportedVersion))

	dc := relay.NewDeviceConn(deviceID, c)
	dc.SetCommandObserver(func(rec relay.CommandRecord) {
		if h.Events != nil {
			h.Events.Append(rec.DeviceID, events.Event{
				Kind:       events.KindCommand,
				Method:     rec.Method,
				Source:     rec.Source,
				DurationMs: rec.Duration.Milliseconds(),
				Outcome:    rec.Outcome,
				Detail:     rec.Detail,
			})
		}
		h.Activity.Record(store.Activity{
			AccountID: accountID, DeviceID: rec.DeviceID,
			Kind: activity.KindCommand, Method: rec.Method, Source: rec.Source,
			Outcome: rec.Outcome, DurationMs: rec.Duration.Milliseconds(), Detail: rec.Detail,
		})
	})
	if h.Events != nil {
		h.Events.Append(deviceID, events.Event{Kind: events.KindConnected})
	}
	h.Activity.Record(store.Activity{AccountID: accountID, DeviceID: deviceID, Kind: activity.KindConnected})
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
	h.Activity.Record(store.Activity{
		AccountID: accountID, DeviceID: deviceID,
		Kind: activity.KindDisconnected, DurationMs: uptime.Milliseconds(), Detail: reason,
	})
}

// versionLabel renders a reported client version for the connect log, naming the
// absence explicitly so an old (non-reporting) client is distinguishable from a
// gap in the logs.
func versionLabel(v string) string {
	if v == "" {
		return "version unknown"
	}
	return "v" + v
}
