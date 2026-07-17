package config

import (
	"os"
	"path/filepath"
)

// Config holds Sutra's runtime configuration. Slice 1 covers the daemon's
// listen address and DB path plus the client's host; the bearer token is
// wired in slice 2.
type Config struct {
	ListenAddr string // daemon bind address, e.g. ":8422"
	DBPath     string // SQLite file path
	Host       string // client target, e.g. "http://localhost:8422"
	Token      string // bearer token (slice 2)
}

// Load builds a Config from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		ListenAddr: envOr("SUTRA_LISTEN", ":8422"),
		DBPath:     envOr("SUTRA_DB", defaultDBPath()),
		Host:       envOr("SUTRA_HOST", "http://localhost:8422"),
		Token:      os.Getenv("SUTRA_TOKEN"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "sutra.db"
	}
	return filepath.Join(home, ".sutra", "sutra.db")
}
