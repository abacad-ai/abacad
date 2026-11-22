// Package inventory owns the device list and its live reachability state.
//
// Config is the durable source of truth; this package adds the in-memory status
// that is cheap to recompute after a restart (online, banner, last error).
package inventory

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"abacad/internal/config"
	"abacad/internal/rfb"
)

// View is a device plus its live state. Password is never serialized because
// config.Device hides it; Token is only returned to an authenticated caller.
type View struct {
	config.Device
	Online    bool      `json:"online"`
	Known     bool      `json:"known"`
	Banner    string    `json:"banner,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
	Warning   string    `json:"warning,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	NextCheck time.Time `json:"next_check,omitempty"`
}

type Status struct {
	Online       bool      `json:"online"`
	Known        bool      `json:"known"`
	Banner       string    `json:"banner,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	LastSeen     time.Time `json:"last_seen,omitempty"`
	Warning      string    `json:"warning,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	Failures     int       `json:"failures,omitempty"`      // consecutive failed checks (network or auth)
	AuthFailures int       `json:"auth_failures,omitempty"` // consecutive auth failures; these count on the device
	LastCheck    time.Time `json:"last_check,omitempty"`
	NextCheck    time.Time `json:"next_check,omitempty"`
	Checking     bool      `json:"checking,omitempty"`

	// Set only on the manual Probe response when the five-minute floor has not
	// elapsed. It is not stored, so the card keeps its last real result.
	Throttled       bool   `json:"throttled,omitempty"`
	ThrottleMessage string `json:"throttle_message,omitempty"`
}

// AddRequest is the input to Add.
type AddRequest struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Auth          string `json:"auth"`
	Password      string `json:"password"`
	Platform      string `json:"platform"`
	Mode          string `json:"mode"`
	Expose        bool   `json:"expose"`
	Health        string `json:"health"`
	HealthCommand string `json:"health_command"`
}

// UpdateRequest is a sparse update; nil fields are left alone.
type UpdateRequest struct {
	Name          *string `json:"name"`
	Host          *string `json:"host"`
	Port          *int    `json:"port"`
	Username      *string `json:"username"`
	Auth          *string `json:"auth"`
	Password      *string `json:"password"`
	Platform      *string `json:"platform"`
	Mode          *string `json:"mode"`
	Expose        *bool   `json:"expose"`
	Health        *string `json:"health"`
	HealthCommand *string `json:"health_command"`
}

type Inventory struct {
	mu       sync.RWMutex
	cfg      *config.Config
	status   map[string]Status
	onChange func()
}

func New(cfg *config.Config) *Inventory {
	return &Inventory{cfg: cfg, status: map[string]Status{}}
}

// OnChange installs a callback fired after a successful mutation, outside the
// inventory lock. The hub uses it to start/stop per-device listeners.
func (in *Inventory) OnChange(fn func()) { in.onChange = fn }

func (in *Inventory) Config() *config.Config { return in.cfg }

func (in *Inventory) Devices() []View {
	in.mu.RLock()
	defer in.mu.RUnlock()
	out := make([]View, 0, len(in.cfg.Devices))
	for _, d := range in.cfg.Devices {
		out = append(out, viewOf(d, in.status[d.ID]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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
	return View{
		Device:    d,
		Online:    s.Online,
		Known:     s.Known,
		Banner:    s.Banner,
		Detail:    s.Detail,
		LastSeen:  s.LastSeen,
		Warning:   s.Warning,
		LastError: s.LastError,
		NextCheck: s.NextCheck,
	}
}

// Add validates, optionally allocates a raw-VNC port, checks the device once,
// and saves. A failed check does not reject the device: the host may still be
// booting, and the UI can show the error.
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
	port := 0
	var err error
	if req.Expose && mode != config.ModeDirect {
		port, err = in.allocatePortLocked()
	}
	if err != nil {
		in.mu.Unlock()
		return View{}, err
	}
	d, err := in.cfg.AddDevice(config.Device{
		Name:          req.Name,
		Host:          req.Host,
		Port:          req.Port,
		Username:      req.Username,
		Auth:          req.Auth,
		Password:      req.Password,
		Platform:      req.Platform,
		Mode:          mode,
		VNCPort:       port,
		Health:        req.Health,
		HealthCommand: req.HealthCommand,
	})
	if err != nil {
		in.mu.Unlock()
		return View{}, err
	}
	in.status[d.ID] = Status{LastError: "checking"}
	in.mu.Unlock()

	in.checkNow(ctx, d.ID)
	if in.onChange != nil {
		in.onChange()
	}
	v, _ := in.Device(d.ID)
	return v, nil
}

func (in *Inventory) Update(ctx context.Context, id string, req UpdateRequest) (View, error) {
	in.mu.Lock()
	var allocate, release bool
	if req.Mode != nil {
		switch *req.Mode {
		case config.ModeGateway, config.ModeL4, config.ModeDirect:
		default:
			in.mu.Unlock()
			return View{}, fmt.Errorf("invalid mode %q", *req.Mode)
		}
	}
	if req.Expose != nil {
		cur, ok := in.cfg.Device(id)
		if !ok {
			in.mu.Unlock()
			return View{}, fmt.Errorf("device %q not found", id)
		}
		if *req.Expose && cur.VNCPort == 0 {
			allocate = true
		}
		if !*req.Expose && cur.VNCPort != 0 {
			release = true
		}
	}
	port := 0
	if allocate {
		p, err := in.allocatePortLocked()
		if err != nil {
			in.mu.Unlock()
			return View{}, err
		}
		port = p
	}
	connChanged := req.Host != nil || req.Port != nil || req.Password != nil ||
		req.Auth != nil || req.Username != nil || req.Health != nil || req.HealthCommand != nil
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
		if req.Health != nil {
			d.Health = strings.TrimSpace(*req.Health)
		}
		if req.HealthCommand != nil {
			d.HealthCommand = strings.TrimSpace(*req.HealthCommand)
			if req.Health == nil && d.HealthCommand != "" && d.Health == config.HealthSession {
				// A caller that supplies only a command clearly wants command
				// health; do not leave it in passive mode by accident.
				d.Health = config.HealthCommand
			}
		}
		// A device without an explicit health mode falls back to the command
		// when one is set, otherwise to passive session-derived state.
		if d.Health == "" {
			if d.HealthCommand != "" {
				d.Health = config.HealthCommand
			} else {
				d.Health = config.HealthSession
			}
		}
		if allocate {
			d.VNCPort = port
		}
		if release {
			d.VNCPort = 0
		}
		if d.Mode == config.ModeDirect {
			d.VNCPort = 0
		}
		return nil
	})
	if err != nil {
		in.mu.Unlock()
		return View{}, err
	}
	if in.status == nil {
		in.status = map[string]Status{}
	}
	if connChanged {
		// New address or credential: allow an immediate check and clear the
		// failure history so a corrected password is not stuck behind backoff.
		st := in.status[id]
		st.Failures = 0
		st.AuthFailures = 0
		st.LastCheck = time.Time{}
		st.NextCheck = time.Time{}
		st.Checking = false
		in.status[id] = st
	}
	in.mu.Unlock()

	if connChanged {
		in.checkNow(ctx, id)
	}
	if in.onChange != nil {
		in.onChange()
	}
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
	if in.onChange != nil {
		in.onChange()
	}
	return nil
}

// Probe runs a user-initiated check. With a health command configured it runs
// that command; otherwise it performs one complete authenticated RFB handshake,
// rate-limited to one every five minutes. It never opens a bare TCP connection.
func (in *Inventory) Probe(ctx context.Context, id string) Status {
	return in.checkDevice(ctx, id, true)
}

func (in *Inventory) checkNow(ctx context.Context, id string) {
	_ = in.checkDevice(ctx, id, true)
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
	st.Failures = 0
	st.AuthFailures = 0
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
	now := time.Now().UTC()
	in.mu.Lock()
	defer in.mu.Unlock()
	st := in.status[id]
	st.Online = false
	st.Known = true
	st.LastError = describeError(err)
	if rfb.IsTooManySecurityFailures(err) {
		st.AuthFailures = vncMaxAuthFailures
		st.NextCheck = now.Add(vncFailureCooldown)
	}
	in.status[id] = st
}

func describeError(err error) string {
	if rfb.IsTooManySecurityFailures(err) {
		return rfb.TooManySecurityFailuresHint
	}
	return err.Error()
}

// allocatePortLocked finds a port in the range that is neither assigned in the
// config nor currently bindable. Caller must hold in.mu.
func (in *Inventory) allocatePortLocked() (int, error) {
	used := map[int]bool{}
	for _, d := range in.cfg.Devices {
		if d.VNCPort != 0 {
			used[d.VNCPort] = true
		}
	}
	for p := in.cfg.VNCPortStart; p <= in.cfg.VNCPortEnd; p++ {
		if used[p] {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free VNC port in %d-%d", in.cfg.VNCPortStart, in.cfg.VNCPortEnd)
}
