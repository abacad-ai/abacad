package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotenvLine(t *testing.T) {
	cases := []struct {
		line, key, val string
		ok             bool
	}{
		{line: `FOO=bar`, key: "FOO", val: "bar", ok: true},
		{line: `  FOO = bar `, key: "FOO", val: "bar", ok: true},
		{line: `export FOO=bar`, key: "FOO", val: "bar", ok: true},
		{line: `FOO="bar baz"`, key: "FOO", val: "bar baz", ok: true},
		{line: `FOO='bar baz'`, key: "FOO", val: "bar baz", ok: true},
		{line: `FOO=`, key: "FOO", val: "", ok: true},
		{line: `URL=https://x/y?a=b`, key: "URL", val: "https://x/y?a=b", ok: true},
		{line: `# comment`, ok: false},
		{line: ``, ok: false},
		{line: `no-equals-sign`, ok: false},
		{line: `=novalue`, ok: false},
	}
	for _, c := range cases {
		key, val, ok := parseDotenvLine(c.line)
		if ok != c.ok || key != c.key || val != c.val {
			t.Errorf("parseDotenvLine(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.line, key, val, ok, c.key, c.val, c.ok)
		}
	}
}

// The real environment must win: production passes values via docker compose
// and a stale .env must never override them.
func TestLoadDotenvDoesNotOverrideRealEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FROM_FILE=file\nALREADY_SET=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ABACAD_ENV_FILE", path)
	t.Setenv("ALREADY_SET", "real")

	loadDotenv()

	if got := os.Getenv("FROM_FILE"); got != "file" {
		t.Errorf("FROM_FILE = %q, want %q", got, "file")
	}
	if got := os.Getenv("ALREADY_SET"); got != "real" {
		t.Errorf("ALREADY_SET = %q, want %q (the file must not override it)", got, "real")
	}
	os.Unsetenv("FROM_FILE")
}

func TestLoadDotenvMissingFileIsNotAnError(t *testing.T) {
	t.Setenv("ABACAD_ENV_FILE", filepath.Join(t.TempDir(), "does-not-exist"))
	loadDotenv() // must not panic
}

func TestFindDotenvWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "server", "backend")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	got, err := filepath.EvalSymlinks(findDotenv())
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("findDotenv() = %q, want %q", got, want)
	}
}
