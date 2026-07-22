// Package agent is the abacad Linux device client: it dials the relay's /device
// WebSocket, dispatches command frames against the X11 backend, and answers the
// binary tunnel lane. It is the headless analogue of the macOS menu-bar app's
// Agent coordinator.
package agent

import (
	"context"
	"encoding/json"
	"log"

	"abacad-linux/internal/x11"
)

// Agent owns the socket, the command dispatcher, and the tunnel.
type Agent struct {
	ws     *wsClient
	disp   *dispatcher
	tunnel *Tunnel
}

// New wires an agent for the given relay URL over the X11 connection.
func New(serverURL string, x *x11.Conn) (*Agent, error) {
	ws, err := newWSClient(serverURL)
	if err != nil {
		return nil, err
	}
	a := &Agent{
		ws:     ws,
		disp:   newDispatcher(x, newBlobClient(ws.blobBaseURL(), ws.token)),
		tunnel: newTunnel(ws.sendBinary),
	}
	ws.onText = a.handleText
	ws.onBinary = a.tunnel.handle
	ws.onState = func(up bool) {
		if up {
			log.Printf("device online")
		} else {
			log.Printf("device offline")
			a.tunnel.closeAll()
		}
	}
	return a, nil
}

// Run services the connection until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	a.ws.run(ctx)
}

// handleText parses a command frame and dispatches it; the reply is correlated
// by id. A malformed frame is dropped with no reply, matching the other clients.
// Each command runs on its own goroutine so a slow one (e.g. a drag with a hold)
// doesn't stall the read loop; the dispatcher itself serializes execution.
func (a *Agent) handleText(text string) {
	var cmd struct {
		ID     string         `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(text), &cmd); err != nil {
		return // malformed → no reply
	}
	go func() {
		result, err := a.disp.execute(cmd.Method, cmd.Params)
		var reply map[string]any
		if err != nil {
			reply = map[string]any{"id": cmd.ID, "ok": false, "error": err.Error()}
		} else {
			reply = map[string]any{"id": cmd.ID, "ok": true, "result": result}
		}
		b, err := json.Marshal(reply)
		if err != nil {
			log.Printf("marshal reply: %v", err)
			return
		}
		a.ws.sendText(string(b))
	}()
}
