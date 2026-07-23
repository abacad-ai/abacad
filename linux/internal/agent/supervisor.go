package agent

import (
	"context"
	"sync"

	"abacad-linux/internal/status"
	"abacad-linux/internal/x11"
)

// Supervisor manages the connection lifecycle for the GUI: connect to a relay
// URL, disconnect, reconnect — each running the agent on a background goroutine
// whose context the Supervisor can cancel. The headless daemon doesn't need it
// (it runs one agent until the process context ends); the GUI does, so its
// Connect / Disconnect buttons map to real actions.
type Supervisor struct {
	x *x11.Conn

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewSupervisor returns a Supervisor that drives agents over the X11 connection x
// (may be nil on a box with no display — the tunnel/SSH verbs still work).
func NewSupervisor(x *x11.Conn) *Supervisor { return &Supervisor{x: x} }

// Connect drops any live connection and dials url on a background goroutine.
func (s *Supervisor) Connect(url string) {
	s.Disconnect()
	if url == "" {
		status.SetState(status.Disconnected, "no server URL set")
		return
	}
	a, err := New(url, s.x)
	if err != nil {
		status.SetState(status.Disconnected, "config error: "+err.Error())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	status.SetState(status.Connecting, "connecting")
	go a.Run(ctx)
}

// Disconnect tears down any live connection.
func (s *Supervisor) Disconnect() {
	s.mu.Lock()
	c := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if c != nil {
		c()
		status.SetState(status.Disconnected, "disconnected")
	}
}
