// Package config loads and saves the single abacad configuration file.
//
// The file is deliberately human-readable YAML: a self-hosted hub is a small
// tool, and a database would be more machinery than the job needs. Passwords
// live in the same file for v0.1 (mode 0600); moving them to the OS keychain is
// a later, local change that does not affect the wire protocol.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultListen = "0.0.0.0:8787"

	// Container deployments configure the hub through the environment
	// instead of editing YAML inside a volume. An unset variable is
	// ignored; flags still win over all of them.
	EnvConfig     = "ABACAD_CONFIG"
	EnvListen     = "ABACAD_LISTEN"
	EnvAdminToken = "ABACAD_ADMIN_TOKEN"

	// MinAdminTokenLen is the shortest admin token ApplyEnv accepts. The
	// generated token is 64 hex characters; a hand-written one should at
	// least not be guessable.
	MinAdminTokenLen = 16

	ModeGateway = "gateway" // hub terminates RFB auth and injects the device password
	ModeL4      = "l4"      // blind TCP forwarding; the client authenticates to the device
	ModeDirect  = "direct"  // catalog only; the hub carries no VNC traffic

	// AuthVNC is the classic 8-byte VNC password (RFB security type 2).
	AuthVNC = "vnc"
	// AuthARD is Apple's Diffie-Hellman account sign-in (RFB security type 30).
	// On macOS this is what attaches to the logged-in console session; the VNC
	// password path has no user identity and can land on the login window.
	AuthARD = "ard"
)

// Device is one VNC server the hub knows about.
type Device struct {
	ID        string    `yaml:"id" json:"id"`
	Name      string    `yaml:"name" json:"name"`
	Host      string    `yaml:"host" json:"host"`
	Port      int       `yaml:"port" json:"port"`
	Username  string    `yaml:"username,omitempty" json:"username,omitempty"`
	Auth      string    `yaml:"auth,omitempty" json:"auth,omitempty"`
	Password  string    `yaml:"password,omitempty" json:"-"`
	Platform  string    `yaml:"platform,omitempty" json:"platform,omitempty"`
	Mode      string    `yaml:"mode,omitempty" json:"mode"`
	CreatedAt time.Time `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

// Config is the whole persisted state of a hub.
type Config struct {
	Listen       string   `yaml:"listen"`
	AdminToken   string   `yaml:"admin_token"`
	CookieSecret string   `yaml:"cookie_secret"`
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

// ApplyEnv applies the environment overrides used by container images.
// getenv is os.Getenv in production; tests pass a map lookup.
//
// Overrides are runtime-only: the YAML file stays the record of what someone
// configured by hand, so removing the variable restores the file's value and
// a secret passed as ABACAD_ADMIN_TOKEN is never written to disk. Command-line
// flags are applied after this and therefore win.
func (c *Config) ApplyEnv(getenv func(string) string) error {
	if v := strings.TrimSpace(getenv(EnvListen)); v != "" {
		if _, _, err := net.SplitHostPort(v); err != nil {
			return fmt.Errorf("%s: %w", EnvListen, err)
		}
		c.Listen = v
	}
	if v := strings.TrimSpace(getenv(EnvAdminToken)); v != "" {
		if len(v) < MinAdminTokenLen {
			return fmt.Errorf("%s must be at least %d characters", EnvAdminToken, MinAdminTokenLen)
		}
		c.AdminToken = v
	}
	return nil
}

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
		d.Username = strings.TrimSpace(d.Username)
		if d.Port == 0 {
			d.Port = 5900
		}
		if d.Mode == "" {
			d.Mode = ModeGateway
		}
		if d.Auth == "" {
			d.Auth = AuthVNC
		}
		if d.ID == "" {
			d.ID = slug(d.Name)
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
		switch d.Auth {
		case AuthVNC, AuthARD:
		default:
			return fmt.Errorf("device %q: invalid auth %q", d.ID, d.Auth)
		}
		if d.Auth == AuthARD && d.Username == "" {
			return fmt.Errorf("device %q: macOS account name is required for auth %q", d.ID, d.Auth)
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
	d.Username = strings.TrimSpace(d.Username)
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
	if d.Auth == "" {
		d.Auth = AuthVNC
	}
	if d.ID == "" {
		d.ID = slug(d.Name)
	}
	if _, ok := c.Device(d.ID); ok {
		d.ID = uniqueID(c, d.ID)
	}
	d.CreatedAt = time.Now().UTC()
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

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
