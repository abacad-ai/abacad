// Package config loads and saves the single abacad configuration file.
//
// The file is deliberately human-readable YAML: a LAN hub is a small tool, and
// a database would be more machinery than the job needs. Passwords live in the
// same file for v0.1 (mode 0600); moving them to the OS keychain is a later,
// local change that does not affect the wire protocol.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultListen       = "0.0.0.0:8787"
	DefaultVNCPortStart = 5901
	DefaultVNCPortEnd   = 5999

	ModeGateway = "gateway" // hub terminates RFB auth and injects the device password
	ModeL4      = "l4"      // blind TCP forwarding; the client authenticates to the device
	ModeDirect  = "direct"  // catalog only; the hub carries no VNC traffic
)

// Device is one VNC server the hub knows about.
type Device struct {
	ID        string    `yaml:"id" json:"id"`
	Name      string    `yaml:"name" json:"name"`
	Host      string    `yaml:"host" json:"host"`
	Port      int       `yaml:"port" json:"port"`
	Password  string    `yaml:"password,omitempty" json:"-"`
	Platform  string    `yaml:"platform,omitempty" json:"platform,omitempty"`
	Mode      string    `yaml:"mode,omitempty" json:"mode"`
	VNCPort   int       `yaml:"vnc_port,omitempty" json:"vnc_port,omitempty"`
	Token     string    `yaml:"token,omitempty" json:"token,omitempty"`
	CreatedAt time.Time `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

// Config is the whole persisted state of a hub.
type Config struct {
	Listen       string   `yaml:"listen"`
	AdminToken   string   `yaml:"admin_token"`
	CookieSecret string   `yaml:"cookie_secret"`
	MDNS         *bool    `yaml:"mdns,omitempty"`
	VNCPortStart int      `yaml:"vnc_port_start"`
	VNCPortEnd   int      `yaml:"vnc_port_end"`
	Devices      []Device `yaml:"devices"`

	path string
}

// DefaultPath is ~/.config/abacad/config.yaml on every platform. Keeping one
// documented path is friendlier than chasing each OS's preferred location.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "abacad.yaml"
	}
	return filepath.Join(home, ".config", "abacad", "config.yaml")
}

// Load reads path, creating a fresh config on first run. An empty path uses
// DefaultPath.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	c := &Config{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := c.normalize(); err != nil {
			return nil, err
		}
		if err := c.Save(); err != nil {
			return nil, err
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Path is the file this config was loaded from.
func (c *Config) Path() string { return c.path }

// MDNSEnabled reports whether discovery is on. The default is on; an explicit
// "mdns: false" in the file turns it off.
func (c *Config) MDNSEnabled() bool { return c.MDNS == nil || *c.MDNS }

// New creates an in-memory default config. The caller normally follows with
// Save.
func New() (*Config, error) {
	c := &Config{}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	c.path = DefaultPath()
	return c, nil
}

func (c *Config) normalize() error {
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	if c.VNCPortStart == 0 {
		c.VNCPortStart = DefaultVNCPortStart
	}
	if c.VNCPortEnd == 0 {
		c.VNCPortEnd = DefaultVNCPortEnd
	}
	if c.VNCPortStart < 1 || c.VNCPortEnd > 65535 || c.VNCPortStart > c.VNCPortEnd {
		return fmt.Errorf("invalid vnc port range %d-%d", c.VNCPortStart, c.VNCPortEnd)
	}
	if c.AdminToken == "" {
		c.AdminToken = randomHex(32)
	}
	if c.CookieSecret == "" {
		c.CookieSecret = randomHex(32)
	}
	for i := range c.Devices {
		d := &c.Devices[i]
		d.Name = strings.TrimSpace(d.Name)
		d.Host = strings.TrimSpace(d.Host)
		if d.Port == 0 {
			d.Port = 5900
		}
		if d.Mode == "" {
			d.Mode = ModeGateway
		}
		if d.ID == "" {
			d.ID = slug(d.Name)
		}
		if d.Token == "" {
			d.Token = RandomDeviceToken()
		}
		if d.CreatedAt.IsZero() {
			d.CreatedAt = time.Now().UTC()
		}
	}
	return c.Validate()
}

// Validate catches the mistakes that would otherwise fail at runtime.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Listen) == "" {
		return errors.New("listen address is empty")
	}
	ids := map[string]bool{}
	ports := map[int]bool{}
	for i, d := range c.Devices {
		if d.ID == "" {
			return fmt.Errorf("device %d: id is empty", i)
		}
		if ids[d.ID] {
			return fmt.Errorf("duplicate device id %q", d.ID)
		}
		ids[d.ID] = true
		if d.Host == "" {
			return fmt.Errorf("device %q: host is empty", d.ID)
		}
		if d.Port < 1 || d.Port > 65535 {
			return fmt.Errorf("device %q: invalid port %d", d.ID, d.Port)
		}
		switch d.Mode {
		case ModeGateway, ModeL4, ModeDirect:
		default:
			return fmt.Errorf("device %q: invalid mode %q", d.ID, d.Mode)
		}
		if d.VNCPort != 0 {
			if d.VNCPort < c.VNCPortStart || d.VNCPort > c.VNCPortEnd {
				return fmt.Errorf("device %q: vnc port %d outside range %d-%d", d.ID, d.VNCPort, c.VNCPortStart, c.VNCPortEnd)
			}
			if ports[d.VNCPort] {
				return fmt.Errorf("duplicate vnc port %d", d.VNCPort)
			}
			ports[d.VNCPort] = true
		}
	}
	return nil
}

// Save writes the config atomically with 0600 permissions.
func (c *Config) Save() error {
	if c.path == "" {
		c.path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// Device returns a device by id.
func (c *Config) Device(id string) (Device, bool) {
	for _, d := range c.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return Device{}, false
}

// AddDevice validates and appends a device, saving the file.
func (c *Config) AddDevice(d Device) (Device, error) {
	d.Name = strings.TrimSpace(d.Name)
	d.Host = strings.TrimSpace(d.Host)
	if d.Name == "" {
		return Device{}, errors.New("name is required")
	}
	if d.Host == "" {
		return Device{}, errors.New("host is required")
	}
	if d.Port == 0 {
		d.Port = 5900
	}
	if d.Mode == "" {
		d.Mode = ModeGateway
	}
	if d.ID == "" {
		d.ID = slug(d.Name)
	}
	if _, ok := c.Device(d.ID); ok {
		d.ID = uniqueID(c, d.ID)
	}
	if d.Token == "" {
		d.Token = RandomDeviceToken()
	}
	d.CreatedAt = time.Now().UTC()
	if d.VNCPort != 0 {
		if err := c.checkPortFree(d.VNCPort); err != nil {
			return Device{}, err
		}
	}
	c.Devices = append(c.Devices, d)
	if err := c.Validate(); err != nil {
		c.Devices = c.Devices[:len(c.Devices)-1]
		return Device{}, err
	}
	if err := c.Save(); err != nil {
		c.Devices = c.Devices[:len(c.Devices)-1]
		return Device{}, err
	}
	return d, nil
}

// RemoveDevice deletes a device by id.
func (c *Config) RemoveDevice(id string) error {
	for i, d := range c.Devices {
		if d.ID == id {
			c.Devices = append(c.Devices[:i], c.Devices[i+1:]...)
			return c.Save()
		}
	}
	return fmt.Errorf("device %q not found", id)
}

// UpdateDevice applies fn to a device and saves.
func (c *Config) UpdateDevice(id string, fn func(*Device) error) (Device, error) {
	for i := range c.Devices {
		if c.Devices[i].ID != id {
			continue
		}
		before := c.Devices[i]
		if err := fn(&c.Devices[i]); err != nil {
			c.Devices[i] = before
			return Device{}, err
		}
		if err := c.Validate(); err != nil {
			c.Devices[i] = before
			return Device{}, err
		}
		if err := c.Save(); err != nil {
			c.Devices[i] = before
			return Device{}, err
		}
		return c.Devices[i], nil
	}
	return Device{}, fmt.Errorf("device %q not found", id)
}

// AllocatePort returns the first unused port in the configured range and
// verifies that it is currently bindable.
func (c *Config) AllocatePort() (int, error) {
	used := map[int]bool{}
	for _, d := range c.Devices {
		if d.VNCPort != 0 {
			used[d.VNCPort] = true
		}
	}
	for p := c.VNCPortStart; p <= c.VNCPortEnd; p++ {
		if used[p] {
			continue
		}
		if err := c.checkPortFree(p); err == nil {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free VNC port in %d-%d", c.VNCPortStart, c.VNCPortEnd)
}

func (c *Config) checkPortFree(p int) error {
	for _, d := range c.Devices {
		if d.VNCPort == p {
			return fmt.Errorf("vnc port %d already assigned to %s", p, d.ID)
		}
	}
	return nil
}

func uniqueID(c *Config, base string) string {
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if _, ok := c.Device(id); !ok {
			return id
		}
	}
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "device"
	}
	return out
}

const tokenAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// RandomDeviceToken returns the 8 characters a RFB VNC-auth password can carry.
// It is deliberately not a general-purpose secret; the gateway rate-limits
// failed attempts and the token can be rotated.
func RandomDeviceToken() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = tokenAlphabet[int(b[i])%len(tokenAlphabet)]
	}
	return string(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
