package config

import (
	"os"
	"path/filepath"
	"testing"
)

func testPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.yaml")
}

func TestLoadCreatesDefaults(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != DefaultListen {
		t.Fatalf("listen = %q", c.Listen)
	}
	if len(c.AdminToken) != 64 || len(c.CookieSecret) != 64 {
		t.Fatalf("tokens not generated: %q %q", c.AdminToken, c.CookieSecret)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", fi.Mode().Perm())
	}
}

func TestAddRemoveDevice(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.AddDevice(Device{Name: "Mac mini", Host: "192.168.1.20", Port: 5900, Password: "pw", Platform: "macos", Mode: ModeGateway})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "mac-mini" {
		t.Fatalf("id = %q", d.ID)
	}
	if len(d.Token) != 8 {
		t.Fatalf("token = %q", d.Token)
	}
	if d.VNCPort != 0 {
		t.Fatalf("vnc port = %d", d.VNCPort)
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Devices) != 1 || again.Devices[0].Name != "Mac mini" {
		t.Fatalf("reload = %+v", again.Devices)
	}
	if err := again.RemoveDevice(d.ID); err != nil {
		t.Fatal(err)
	}
	third, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Devices) != 0 {
		t.Fatalf("device survived remove: %+v", third.Devices)
	}
}

func TestDuplicateIDGetsSuffix(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddDevice(Device{Name: "Phone", Host: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	d, err := c.AddDevice(Device{Name: "Phone", Host: "10.0.0.3"})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "phone-2" {
		t.Fatalf("id = %q", d.ID)
	}
}

func TestRandomDeviceToken(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok := RandomDeviceToken()
		if len(tok) != 8 {
			t.Fatalf("token length = %d", len(tok))
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
	}
}

func TestARDAuthDefaultsAndValidation(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.AddDevice(Device{Name: "Mac mini", Host: "192.168.1.20", Username: "li", Auth: AuthARD, Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Auth != AuthARD || d.Username != "li" {
		t.Fatalf("device = %+v", d)
	}
	if _, err := c.AddDevice(Device{Name: "No user", Host: "192.168.1.21", Auth: AuthARD}); err == nil {
		t.Fatal("expected missing macOS username to be rejected")
	}
	legacy, err := c.AddDevice(Device{Name: "Legacy", Host: "192.168.1.22", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Auth != AuthVNC {
		t.Fatalf("legacy auth = %q", legacy.Auth)
	}
}

func TestHealthDefaultsAndValidation(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.AddDevice(Device{Name: "Passive", Host: "10.0.0.2"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Health != HealthSession {
		t.Fatalf("default health = %q", d.Health)
	}
	cmd, err := c.AddDevice(Device{
		Name: "Command", Host: "10.0.0.3",
		HealthCommand: "ssh debian 'systemctl is-active tigervncserver@:2'",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Health != HealthCommand {
		t.Fatalf("command health = %q", cmd.Health)
	}
	if _, err := c.AddDevice(Device{Name: "Bad", Host: "10.0.0.4", Health: "poll"}); err == nil {
		t.Fatal("expected an invalid health mode to be rejected")
	}
	if _, err := c.AddDevice(Device{Name: "Empty", Host: "10.0.0.5", Health: HealthCommand}); err == nil {
		t.Fatal("expected a missing health command to be rejected")
	}
}
