// Package inventory owns the device list and its live reachability state.
//
// Config is the durable source of truth; this package adds the in-memory status
// that is cheap to recompute after a restart (online, banner, last error).
package inventory

import (
	"bufio"
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
	Banner    string    `json:"banner,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
	Warning   string    `json:"warning,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

type Status struct {
	Online      bool
	Banner      string
	LastSeen    time.Time
	Warning     string
	LastError   string
	AuthChecked bool // security-type probe done in this process
}

// AddRequest is the input to Add.
type AddRequest struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
	Expose   bool   `json:"expose"`
}

// UpdateRequest is a sparse update; nil fields are left alone.
type UpdateRequest struct {
	Name     *string `json:"name"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Password *string `json:"password"`
	Platform *string `json:"platform"`
	Mode     *string `json:"mode"`
	Expose   *bool   `json:"expose"`
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
		s := in.status[d.ID]
		out = append(out, View{Device: d, Online: s.Online, Banner: s.Banner, LastSeen: s.LastSeen, Warning: s.Warning, LastError: s.LastError})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (in *Inventory) Device(id string) (View, bool) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	for _, d := range in.cfg.Devices {
		if d.ID == id {
			s := in.status[d.ID]
			return View{Device: d, Online: s.Online, Banner: s.Banner, LastSeen: s.LastSeen, Warning: s.Warning, LastError: s.LastError}, true
		}
	}
	return View{}, false
}

// Add validates, optionally allocates a raw-VNC port, probes once, and saves.
// A failed probe does not reject the device: the host may still be booting, and
// the UI can show the error.
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
		Name:     req.Name,
		Host:     req.Host,
		Port:     req.Port,
		Password: req.Password,
		Platform: req.Platform,
		Mode:     mode,
		VNCPort:  port,
	})
	if err != nil {
		in.mu.Unlock()
		return View{}, err
	}
	in.status[d.ID] = Status{LastError: "probing"}
	in.mu.Unlock()

	in.Probe(ctx, d.ID)
	in.CheckAuth(ctx, d.ID)
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
		if req.Password != nil {
			d.Password = *req.Password
		}
		if req.Platform != nil {
			d.Platform = *req.Platform
		}
		if req.Mode != nil {
			d.Mode = *req.Mode
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
	in.mu.Unlock()

	in.Probe(ctx, id)
	in.CheckAuth(ctx, id)
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

// Probe checks TCP reachability and reads the RFB banner. It deliberately does
// not authenticate: a health check must not consume a password attempt or
// disturb an active session.
func (in *Inventory) Probe(ctx context.Context, id string) Status {
	in.mu.RLock()
	var d config.Device
	found := false
	for _, x := range in.cfg.Devices {
		if x.ID == id {
			d, found = x, true
			break
		}
	}
	in.mu.RUnlock()
	if !found {
		return Status{LastError: "device not found"}
	}

	in.mu.RLock()
	old := in.status[id] // preserve an auth warning across routine probes
	in.mu.RUnlock()

	s := probe(ctx, d)
	s.Warning = old.Warning
	s.AuthChecked = old.AuthChecked
	in.mu.Lock()
	in.status[id] = s
	in.mu.Unlock()
	return s
}

// CheckAuth probes the RFB security types. It is called when a device is added
// or edited, not on every health check: it opens one extra connection and is
// there to turn the most common macOS failure into a readable card warning.
func (in *Inventory) CheckAuth(ctx context.Context, id string) {
	v, ok := in.Device(id)
	if !ok || !v.Online {
		return
	}
	addr := net.JoinHostPort(v.Host, fmt.Sprint(v.Port))
	_, types, err := rfb.SecurityTypes(ctx, addr, 3*time.Second)
	if err != nil {
		return
	}
	warning := ""
	if !rfb.SupportsStandardAuth(types) {
		warning = fmt.Sprintf("device only offers security types %v; enable \"VNC viewers may control screen with password\" or use TigerVNC", types)
	}
	in.mu.Lock()
	st := in.status[id]
	st.Warning = warning
	st.AuthChecked = true
	in.status[id] = st
	in.mu.Unlock()
}

// CheckAuthIfNeeded runs the security-type probe at most once per process (or
// until the next successful probe), so startup restores the macOS warning
// without opening an extra connection every 30 seconds.
func (in *Inventory) CheckAuthIfNeeded(ctx context.Context, id string) {
	in.mu.RLock()
	checked := in.status[id].AuthChecked
	in.mu.RUnlock()
	if checked {
		return
	}
	in.CheckAuth(ctx, id)
}

func probe(ctx context.Context, d config.Device) Status {
	addr := net.JoinHostPort(d.Host, fmt.Sprint(d.Port))
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Status{LastError: err.Error(), LastSeen: time.Now().UTC()}
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	banner, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return Status{LastError: "no RFB banner: " + err.Error(), LastSeen: time.Now().UTC()}
	}
	if len(banner) > 0 && banner[len(banner)-1] == '\n' {
		banner = banner[:len(banner)-1]
	}
	if len(banner) < 7 || banner[:4] != "RFB " {
		return Status{LastError: fmt.Sprintf("unexpected banner %q", banner), LastSeen: time.Now().UTC()}
	}
	return Status{Online: true, Banner: banner, LastSeen: time.Now().UTC()}
}

// ProbeAll probes every device with a small concurrency cap.
func (in *Inventory) ProbeAll(ctx context.Context) {
	devices := in.Devices()
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, d := range devices {
		d := d
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			in.Probe(ctx, d.ID)
			in.CheckAuthIfNeeded(ctx, d.ID)
		}()
	}
	wg.Wait()
}

// Run probes on an interval until ctx is canceled.
func (in *Inventory) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	in.ProbeAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			in.ProbeAll(ctx)
		}
	}
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
