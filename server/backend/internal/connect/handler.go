// Package connect serves the /connect WebSocket: a raw TCP tunnel from an
// authenticated agent-side client to a target host:port reachable *from a
// device*, multiplexed over that device's existing out-dial WebSocket.
//
// This is the "just connected" surface. Instead of teaching an agent a method
// per tool (ssh, rsync, scp, git...), the relay makes the device reachable as a
// host: the client speaks raw bytes, we bridge them to a relay Stream, and the
// device dials the target. The relay never interprets the bytes — an SSH or TLS
// session stays end-to-end encrypted, and the server holds no keys. Authorization
// is at connect time (does this account own this device?), not per byte.
package connect

import (
	"context"
	"net/http"

	"github.com/coder/websocket"

	"abacad/internal/mcp"
)

// readLimit caps one inbound client frame. Clients chunk their writes, so this
// only needs to be generous, not huge.
const readLimit = 16 << 20

// Handler bridges /connect clients to device streams.
type Handler struct {
	// ResolverFor authenticates the request (MCP token -> account-scoped
	// resolver), exactly like the MCP endpoint. Returning an error rejects 401.
	ResolverFor func(r *http.Request) (mcp.DeviceResolver, error)
}

// ServeHTTP handles GET /connect?device=<id>&target=<host:port> (token via
// ?token= or Authorization: Bearer, checked by ResolverFor). device may be empty
// to use the account's default (sole / most-recently-active online) device.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resolver, err := h.ResolverFor(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="abacad"`)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "missing target host:port", http.StatusBadRequest)
		return
	}
	dc, err := resolver.Resolve(r.Context(), r.URL.Query().Get("device"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	stream, err := dc.OpenStream(r.Context(), target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer stream.Close()

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // non-browser client (abacad proxy); no Origin to check
	})
	if err != nil {
		return // Accept already wrote the error
	}
	c.SetReadLimit(readLimit)
	defer c.Close(websocket.StatusNormalClosure, "bye")

	// Bridge both directions; whichever side ends first cancels the other. The
	// client WebSocket has a single reader (this goroutine) and a single writer
	// (the pump below), so it needs no write lock.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() { // device -> client
		defer cancel()
		buf := make([]byte, 32<<10)
		for {
			n, rerr := stream.Read(buf)
			if n > 0 {
				if werr := c.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					return
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	for { // client -> device
		typ, data, rerr := c.Read(ctx)
		if rerr != nil {
			return
		}
		if typ != websocket.MessageBinary {
			continue
		}
		if _, werr := stream.Write(data); werr != nil {
			return
		}
	}
}
