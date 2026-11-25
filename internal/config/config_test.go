package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestApplyEnvOverridesWithoutPersisting(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	original := c.AdminToken

	env := map[string]string{
		EnvListen:     "127.0.0.1:9000",
		EnvAdminToken: "0123456789abcdef",
	}
	if err := c.ApplyEnv(func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:9000" {
		t.Fatalf("listen = %q", c.Listen)
	}
	if c.AdminToken != "0123456789abcdef" {
		t.Fatalf("admin token = %q", c.AdminToken)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AdminToken != original {
		t.Fatal("environment token was written to disk")
	}
	if reloaded.Listen != DefaultListen {
		t.Fatalf("environment listen was written to disk: %q", reloaded.Listen)
	}
}

func TestApplyEnvRejectsBadValues(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := []map[string]string{
		{EnvListen: "not-an-address"},
		{EnvAdminToken: "short"},
	}
	for _, env := range cases {
		if err := c.ApplyEnv(func(k string) string { return env[k] }); err == nil {
			t.Fatalf("expected %v to be rejected", env)
		}
	}
}

func TestApplyEnvIgnoresEmpty(t *testing.T) {
	path := testPath(t)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	before := *c
	if err := c.ApplyEnv(func(string) string { return "  " }); err != nil {
		t.Fatal(err)
	}
	if c.Listen != before.Listen || c.AdminToken != before.AdminToken {
		t.Fatal("empty environment changed the config")
	}
}

// Configs written before raw VNC ports were removed still carry vnc_port,
// token and the port range. They must load, and the next save drops them.
func TestLoadIgnoresRemovedRawPortFields(t *testing.T) {
	path := testPath(t)
	legacy := "listen: 0.0.0.0:8787\nadmin_token: 0123456789abcdef0123\ncookie_secret: s\n" +
		"vnc_port_start: 5901\nvnc_port_end: 5999\ndevices:\n" +
		"  - id: s10\n    name: S10\n    host: 172.16.1.43\n    port: 5900\n    vnc_port: 5901\n    token: Abcd1234\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Devices) != 1 || c.Devices[0].ID != "s10" {
		t.Fatalf("devices = %+v", c.Devices)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"vnc_port", "token: Abcd1234"} {
		if strings.Contains(string(b), gone) {
			t.Fatalf("saved config still has %q:\n%s", gone, b)
		}
	}
}
