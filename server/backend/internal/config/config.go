// Package config holds runtime configuration, sourced from flags and env.
package config

import (
	"flag"
	"os"
	"strconv"
)

type Config struct {
	Addr         string // listen address, e.g. ":8848"
	DBPath       string // SQLite file path
	BlobDir      string // directory for data-plane blob bytes (/blobs store)
	MaxBlobBytes int64  // reject a single blob upload larger than this
	DevCORS      bool   // permissive CORS for local dev (Vite / smoke.mjs hitting Go directly)
	Seed         bool   // create a dev account/device/tokens on boot
}

// Load reads flags (which fall back to env) and returns the config.
func Load() Config {
	var c Config
	flag.StringVar(&c.Addr, "addr", envOr("ABACAD_ADDR", ":8848"), "listen address")
	flag.StringVar(&c.DBPath, "db", envOr("ABACAD_DB", "abacad.db"), "SQLite database path")
	flag.StringVar(&c.BlobDir, "blobs", envOr("ABACAD_BLOBS", "abacad-blobs"), "directory for /blobs data-plane storage")
	flag.Int64Var(&c.MaxBlobBytes, "max-blob-bytes", envOrInt64("ABACAD_MAX_BLOB_BYTES", 1<<30), "reject a single /blobs upload larger than this (bytes)")
	flag.BoolVar(&c.DevCORS, "dev-cors", os.Getenv("ABACAD_DEV_CORS") == "1", "enable permissive CORS for local dev")
	flag.BoolVar(&c.Seed, "seed", false, "seed a dev account/device/tokens on boot and print them")
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
