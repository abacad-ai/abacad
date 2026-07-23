// Package agent is the abacad Linux device client: it dials the relay's /device
// WebSocket, dispatches command frames against the X11 backend, and answers the
// binary tunnel lane. It is the headless analogue of the macOS menu-bar app's
// Agent coordinator.
package agent

import (
	"context"
	"encoding/json"
	"log"

	"abacad-linux/internal/status"
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
			// Headless: the log is the only disclosure surface, so state it plainly.
			log.Printf("device online — this machine can now be viewed and controlled remotely by an agent")
			status.SetState(status.Connected, "connected")
		} else {
			log.Printf("device offline")
			status.SetState(status.Disconnected, "disconnected")
			a.tunnel.closeAll()
		}
	}
	return a, nil
}

// updateAwareness reflects live-view / recording sessions in the status panel so
// the person at the machine sees when their screen is being watched or recorded.
// Best-effort, inferred from the command verbs the relay sends.
func updateAwareness(method string, params map[string]any) {
	action, _ := params["action"].(string)
	switch method {
	case "vnc":
		switch action {
		case "start":
			status.SetWatched(true)
		case "stop":
			status.SetWatched(false)
		}
	case "screen_recording":
		switch action {
		case "start":
			status.SetRecording(true)
		case "stop":
			status.SetRecording(false)
		}
	}
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
	// Soft-kill: while the operator has paused control from the GUI, reject every
	// command locally without touching the machine. The agent sees an error; only
	// the GUI can clear the pause. This is the on-device stop.
	if status.Paused() {
		status.Event(cmd.Method + " · rejected · paused")
		reply := map[string]any{"id": cmd.ID, "ok": false, "error": "paused by device operator"}
		if b, err := json.Marshal(reply); err == nil {
			a.ws.sendText(string(b))
		}
		return
	}
	status.NoteCommand(cmd.Method)
	updateAwareness(cmd.Method, cmd.Params)
	go func() {
		result, err := a.disp.execute(cmd.Method, cmd.Params)
		var reply map[string]any
		if err != nil {
			status.Event(cmd.Method + " · error · " + err.Error())
			reply = map[string]any{"id": cmd.ID, "ok": false, "error": err.Error()}
		} else {
			status.Event(cmd.Method + " · ok")
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
