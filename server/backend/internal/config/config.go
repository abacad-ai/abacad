// Package config holds runtime configuration, sourced from flags and env.
package config

import (
	"flag"
	"os"
)

type Config struct {
	Addr   string // listen address, e.g. ":8848"
	DBPath string // SQLite file path
	DevCORS bool  // permissive CORS for local dev (Vite / smoke.mjs hitting Go directly)
	Seed   bool   // create a dev account/device/tokens on boot
}

// Load reads flags (which fall back to env) and returns the config.
func Load() Config {
	var c Config
	flag.StringVar(&c.Addr, "addr", envOr("ABACAD_ADDR", ":8848"), "listen address")
	flag.StringVar(&c.DBPath, "db", envOr("ABACAD_DB", "abacad.db"), "SQLite database path")
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
