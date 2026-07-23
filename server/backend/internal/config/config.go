// Package config holds runtime configuration, sourced from flags and env.
package config

import (
	"flag"
	"os"
	"strconv"
)

type Config struct {
	Addr          string // listen address, e.g. ":8848"
	DBPath        string // SQLite file path
	BlobDir       string // directory for data-plane blob bytes (/blobs store)
	ScreenshotDir string // directory for per-device last-screenshot bytes
	DownloadsDir  string // directory of public release artifacts served at /downloads/
	MaxBlobBytes  int64  // reject a single blob upload larger than this
	DevCORS       bool   // permissive CORS for local dev (Vite / smoke.mjs hitting Go directly)
	Seed          bool   // create a dev account/device/tokens on boot

	ActivityRetentionDays int // prune activity-trail rows older than this (0 = keep forever)

	// SSH jump host (ssh <device>.<base-domain> via ProxyJump). Disabled when
	// SSHAddr is empty, so local dev and tests opt in explicitly.
	SSHAddr    string // SSH jump listen address(es), comma-separated e.g. ":22,:443" (empty = disabled)
	SSHHostKey string // path to the jump's persistent host key (created if absent)
	BaseDomain string // domain devices hang off, e.g. "abacad.ai"

	// Signed /blobs capability URLs (send_file / get_file). BlobSigningKey is the
	// HMAC key; empty means "generate a random one at boot" (fine for a single
	// instance, set it explicitly for persistence across restarts / multi-instance).
	// PublicBaseURL is the scheme+host the minted URLs point at; empty derives
	// https://<BaseDomain>, and it is overridable for local testing.
	BlobSigningKey string
	PublicBaseURL  string

	// Google OAuth ("Sign in with Google"). Disabled unless both the client id
	// and secret are set; RedirectURL is optional and derived from the incoming
	// request when empty (<origin>/api/auth/google/callback).
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
}

// GoogleEnabled reports whether "Sign in with Google" is configured.
func (c Config) GoogleEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

// Load reads flags (which fall back to env, which falls back to a .env file)
// and returns the config.
func Load() Config {
	loadDotenv()

	var c Config
	flag.StringVar(&c.Addr, "addr", envOr("ABACAD_ADDR", ":8848"), "listen address")
	flag.StringVar(&c.DBPath, "db", envOr("ABACAD_DB", "abacad.db"), "SQLite database path")
	flag.StringVar(&c.BlobDir, "blobs", envOr("ABACAD_BLOBS", "abacad-blobs"), "directory for /blobs data-plane storage")
	flag.StringVar(&c.ScreenshotDir, "screenshots", envOr("ABACAD_SCREENSHOTS", "abacad-screenshots"), "directory for per-device last-screenshot storage")
	flag.StringVar(&c.DownloadsDir, "downloads", envOr("ABACAD_DOWNLOADS", "abacad-downloads"), "directory of public release artifacts served at /downloads/")
	flag.Int64Var(&c.MaxBlobBytes, "max-blob-bytes", envOrInt64("ABACAD_MAX_BLOB_BYTES", 1<<30), "reject a single /blobs upload larger than this (bytes)")
	flag.BoolVar(&c.DevCORS, "dev-cors", os.Getenv("ABACAD_DEV_CORS") == "1", "enable permissive CORS for local dev")
	flag.BoolVar(&c.Seed, "seed", false, "seed a dev account/device/tokens on boot and print them")
	flag.IntVar(&c.ActivityRetentionDays, "activity-retention-days", int(envOrInt64("ABACAD_ACTIVITY_RETENTION_DAYS", 90)), "prune activity-trail rows older than this many days (0 keeps them forever)")
	flag.StringVar(&c.SSHAddr, "ssh-addr", envOr("ABACAD_SSH_ADDR", ""), "SSH jump host listen address(es), comma-separated e.g. :22,:443 (empty disables it)")
	flag.StringVar(&c.SSHHostKey, "ssh-host-key", envOr("ABACAD_SSH_HOST_KEY", "ssh_host_ed25519_key"), "path to the SSH jump host key (created if absent)")
	flag.StringVar(&c.BaseDomain, "base-domain", envOr("ABACAD_BASE_DOMAIN", "abacad.ai"), "domain devices are addressed under (ssh <device>.<base-domain>)")
	flag.StringVar(&c.BlobSigningKey, "blob-signing-key", envOr("ABACAD_BLOB_SIGNING_KEY", ""), "HMAC key for signed /blobs capability URLs (empty generates a random key at boot)")
	flag.StringVar(&c.PublicBaseURL, "public-base-url", envOr("ABACAD_PUBLIC_BASE_URL", ""), "scheme+host that minted signed URLs point at (empty derives https://<base-domain>)")
	flag.StringVar(&c.GoogleClientID, "google-client-id", envOr("ABACAD_GOOGLE_CLIENT_ID", ""), "Google OAuth client ID (enables 'Sign in with Google' when set together with the secret)")
	flag.StringVar(&c.GoogleClientSecret, "google-client-secret", envOr("ABACAD_GOOGLE_CLIENT_SECRET", ""), "Google OAuth client secret")
	flag.StringVar(&c.GoogleRedirectURL, "google-redirect-url", envOr("ABACAD_GOOGLE_REDIRECT_URL", ""), "Google OAuth redirect URL (default: derived from the request as <origin>/api/auth/google/callback)")
	flag.Parse()
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
