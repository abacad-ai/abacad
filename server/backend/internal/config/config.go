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
	ReplayDir     string // directory for session-replay frame bytes
	DownloadsDir  string // directory of public release artifacts served at /downloads/
	MaxBlobBytes  int64  // reject a single blob upload larger than this
	DevCORS       bool   // permissive CORS for local dev (Vite / smoke.mjs hitting Go directly)
	Seed          bool   // create a dev account/device/tokens on boot

	// GeoIPDB is the path to a MaxMind GeoLite2/GeoIP2 *City* database. Empty
	// disables geo: activity rows then carry an IP but no country or city. The
	// database is not shipped with abacad — MaxMind's terms don't permit
	// redistributing it — so operators download their own and keep it updated.
	GeoIPDB string

	ActivityRetentionDays    int // prune activity-trail rows older than this (0 = keep forever)
	BlobRetentionDays        int // delete data-plane blobs (files, recordings) older than this (0 = keep forever)
	ScreenshotRetentionHours int // delete cached per-device screenshots older than this (0 = keep forever)

	// Session replay (off per device until its owner turns it on). Two bounds,
	// because the window alone doesn't bound disk: a single busy agent can record
	// thousands of frames well inside it, so the per-device step cap is the
	// backstop that keeps one device from filling the volume.
	ReplayRetentionHours    int // delete recorded steps and frames older than this (0 = keep forever)
	ReplayMaxStepsPerDevice int // keep at most this many recorded steps per device (0 = unlimited)

	// After an action (tap/click/swipe/…) the recorder takes its own screenshot,
	// so the replay shows what the action DID and not only the screen it acted on.
	// This is the one place an observability feature sends a command to a device,
	// so it has an off switch that does not require turning recording off.
	ReplayCaptureActions bool
	ReplaySettleMs       int // wait this long for the UI to settle before that capture

	// Device enrollment expiry is a fixed product behavior (see api.enrollmentTTL),
	// not an operator knob. This is only the data-retention window for the dead
	// rows it leaves behind — same category as the other *Retention knobs above.
	DeviceDormantDeleteDays int // hard-delete devices this many days after they expire (0 = keep dormant forever)

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
	flag.StringVar(&c.BlobDir, "blobs", envOr("ABACAD_BLOBS", "blobs"), "directory for /blobs data-plane storage")
	flag.StringVar(&c.ScreenshotDir, "screenshots", envOr("ABACAD_SCREENSHOTS", "screenshots"), "directory for per-device last-screenshot storage")
	flag.StringVar(&c.ReplayDir, "replay", envOr("ABACAD_REPLAY", "replay"), "directory for session-replay frame storage")
	flag.StringVar(&c.DownloadsDir, "downloads", envOr("ABACAD_DOWNLOADS", "downloads"), "directory of public release artifacts served at /downloads/")
	flag.Int64Var(&c.MaxBlobBytes, "max-blob-bytes", envOrInt64("ABACAD_MAX_BLOB_BYTES", 1<<30), "reject a single /blobs upload larger than this (bytes)")
	flag.BoolVar(&c.DevCORS, "dev-cors", os.Getenv("ABACAD_DEV_CORS") == "1", "enable permissive CORS for local dev")
	flag.BoolVar(&c.Seed, "seed", false, "seed a dev account/device/tokens on boot and print them")
	flag.StringVar(&c.GeoIPDB, "geoip-db", envOr("ABACAD_GEOIP_DB", ""), "path to a MaxMind GeoLite2/GeoIP2 City .mmdb, to record the country/city an activity came from (empty disables geo)")
	flag.IntVar(&c.ActivityRetentionDays, "activity-retention-days", int(envOrInt64("ABACAD_ACTIVITY_RETENTION_DAYS", 90)), "prune activity-trail rows older than this many days (0 keeps them forever)")
	flag.IntVar(&c.BlobRetentionDays, "blob-retention-days", int(envOrInt64("ABACAD_BLOB_RETENTION_DAYS", 7)), "delete data-plane blobs (transferred files, screen recordings) older than this many days (0 keeps them forever)")
	flag.IntVar(&c.ScreenshotRetentionHours, "screenshot-retention-hours", int(envOrInt64("ABACAD_SCREENSHOT_RETENTION_HOURS", 24)), "delete cached per-device screenshots older than this many hours (0 keeps them forever)")
	flag.IntVar(&c.ReplayRetentionHours, "replay-retention-hours", int(envOrInt64("ABACAD_REPLAY_RETENTION_HOURS", 48)), "delete recorded session-replay steps and frames older than this many hours (0 keeps them forever)")
	flag.IntVar(&c.ReplayMaxStepsPerDevice, "replay-max-steps-per-device", int(envOrInt64("ABACAD_REPLAY_MAX_STEPS_PER_DEVICE", 4000)), "keep at most this many recorded steps per device, dropping the oldest (0 = unlimited)")
	flag.BoolVar(&c.ReplayCaptureActions, "replay-capture-actions", envOrBool("ABACAD_REPLAY_CAPTURE_ACTIONS", true), "on a recording device, take a screenshot after each action so the replay shows what it did")
	flag.IntVar(&c.ReplaySettleMs, "replay-settle-ms", int(envOrInt64("ABACAD_REPLAY_SETTLE_MS", 800)), "how long to let the UI settle before the post-action screenshot (0 disables it)")
	flag.IntVar(&c.DeviceDormantDeleteDays, "device-dormant-delete-days", int(envOrInt64("ABACAD_DEVICE_DORMANT_DELETE_DAYS", 7)), "hard-delete devices this many days after they expire (0 = keep dormant forever)")
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

// envOrBool reads a boolean knob. Unlike the ABACAD_DEV_CORS=1 checks elsewhere,
// this has to distinguish "unset" from "set to off" — the default is on, so
// `=0` must be able to turn it off. An unparseable value keeps the default
// rather than guessing, since guessing "off" would silently disable a feature
// over a typo.
func envOrBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
