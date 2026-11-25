// Package inventory owns the device list and its live reachability state.
//
// Config is the durable source of truth; this package adds the in-memory status
// that is cheap to recompute after a restart (online, banner, last error).
package inventory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"abacad/internal/config"
	"abacad/internal/rfb"
)

// staleAfter is how long a session-derived "online" state stays credible
// without a new authenticated session. After that the UI falls back to
// "last seen" rather than pretending the device is still online.
const staleAfter = 10 * time.Minute

// View is a device plus its live state. Password is never serialized because
// config.Device hides it.
type View struct {
	config.Device
	Online    bool       `json:"online"`
	Known     bool       `json:"known"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
	LastError string     `json:"last_error,omitempty"`
}

// Status is the in-memory, session-derived reachability of one device. There is
// no scheduled check and no probe: Online only becomes true after a real
// browser or agent session authenticates to the device.
type Status struct {
	Online    bool      `json:"online"`
	Known     bool      `json:"known"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

// AddRequest is the input to Add.
type AddRequest struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Auth     string `json:"auth"`
	Password string `json:"password"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

// UpdateRequest is a sparse update; nil fields are left alone.
type UpdateRequest struct {
	Name     *string `json:"name"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Username *string `json:"username"`
	Auth     *string `json:"auth"`
	Password *string `json:"password"`
	Platform *string `json:"platform"`
	Mode     *string `json:"mode"`
}

type Inventory struct {
	mu     sync.RWMutex
	cfg    *config.Config
	status map[string]Status
}

func New(cfg *config.Config) *Inventory {
	return &Inventory{cfg: cfg, status: map[string]Status{}}
}

func (in *Inventory) Config() *config.Config { return in.cfg }

func (in *Inventory) Devices() []View {
	in.mu.RLock()
	defer in.mu.RUnlock()
	out := make([]View, 0, len(in.cfg.Devices))
	for _, d := range in.cfg.Devices {
		out = append(out, viewOf(d, in.status[d.ID]))
	}
	// Keep the order the user configured: the rail is stable across polls and
	// does not reshuffle when a device changes state.
	return out
}

func (in *Inventory) Device(id string) (View, bool) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	for _, d := range in.cfg.Devices {
		if d.ID == id {
			return viewOf(d, in.status[d.ID]), true
		}
	}
	return View{}, false
}

func viewOf(d config.Device, s Status) View {
	online := s.Online
	if online && (s.LastSeen.IsZero() || time.Since(s.LastSeen) > staleAfter) {
		online = false
	}
	var lastSeen *time.Time
	if !s.LastSeen.IsZero() {
		t := s.LastSeen
		lastSeen = &t
	}
	return View{
		Device:    d,
		Online:    online,
		Known:     s.Known,
		LastSeen:  lastSeen,
		LastError: s.LastError,
	}
}

// Add validates and saves. It never
// connects to the device: the UI shows the new machine as "not seen" until a
// real browser or agent session authenticates.
func (in *Inventory) Add(ctx context.Context, req AddRequest) (View, error) {
	mode := req.Mode
	if mode == "" {
		mode = config.ModeGateway
	}
	switch mode {
	case config.ModeGateway, config.ModeL4, config.ModeDirect:
	default:
		return View{}, fmt.Errorf("invalid mode %q", mode)
	}
	if req.Port == 0 {
		req.Port = 5900
	}

	in.mu.Lock()
	d, err := in.cfg.AddDevice(config.Device{
		Name:     req.Name,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Auth:     req.Auth,
		Password: req.Password,
		Platform: req.Platform,
		Mode:     mode,
	})
	in.mu.Unlock()
	if err != nil {
		return View{}, err
	}
	v, _ := in.Device(d.ID)
	return v, nil
}

func (in *Inventory) Update(ctx context.Context, id string, req UpdateRequest) (View, error) {
	in.mu.Lock()
	if req.Mode != nil {
		switch *req.Mode {
		case config.ModeGateway, config.ModeL4, config.ModeDirect:
		default:
			in.mu.Unlock()
			return View{}, fmt.Errorf("invalid mode %q", *req.Mode)
		}
	}
	connChanged := req.Host != nil || req.Port != nil || req.Password != nil ||
		req.Auth != nil || req.Username != nil
	d, err := in.cfg.UpdateDevice(id, func(d *config.Device) error {
		if req.Name != nil {
			d.Name = *req.Name
		}
		if req.Host != nil {
			d.Host = strings.TrimSpace(*req.Host)
		}
		if req.Port != nil {
			d.Port = *req.Port
		}
		if req.Username != nil {
			d.Username = strings.TrimSpace(*req.Username)
		}
		if req.Auth != nil {
			d.Auth = *req.Auth
		}
		if req.Password != nil {
			d.Password = *req.Password
		}
		if req.Platform != nil {
			d.Platform = *req.Platform
		}
		if req.Mode != nil {
			d.Mode = *req.Mode
		}
		return nil
	})
	if err != nil {
		in.mu.Unlock()
		return View{}, err
	}
	if connChanged {
		// A new address or credential invalidates the old reachability: the
		// next observation has to come from a real session against the new
		// endpoint.
		delete(in.status, id)
	}
	in.mu.Unlock()

	v, _ := in.Device(d.ID)
	return v, nil
}

func (in *Inventory) Remove(id string) error {
	in.mu.Lock()
	if err := in.cfg.RemoveDevice(id); err != nil {
		in.mu.Unlock()
		return err
	}
	delete(in.status, id)
	in.mu.Unlock()
	return nil
}

// MarkOnline records a successful authenticated session. It is called after the
// upstream ServerInit, so it never claims a device is online just because TCP
// accepted a connection.
func (in *Inventory) MarkOnline(id string) {
	now := time.Now().UTC()
	in.mu.Lock()
	defer in.mu.Unlock()
	st := in.status[id]
	st.Online = true
	st.Known = true
	st.LastSeen = now
	st.LastError = ""
	in.status[id] = st
}

// SetSessionError records why a session failed after the device was already
// known to be online (for example a downstream token error). It keeps the
// online state.
func (in *Inventory) SetSessionError(id string, err error) {
	if err == nil {
		return
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	st := in.status[id]
	st.LastError = describeError(err)
	in.status[id] = st
}

// MarkOffline records a session failure that happened before the device
// authenticated, so the card can show a readable reason and an offline state.
func (in *Inventory) MarkOffline(id string, err error) {
	if err == nil {
		return
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	st := in.status[id]
	st.Online = false
	st.Known = true
	st.LastError = describeError(err)
	in.status[id] = st
}

func describeError(err error) string {
	if rfb.IsTooManySecurityFailures(err) {
		return rfb.TooManySecurityFailuresHint
	}
	return err.Error()
}
