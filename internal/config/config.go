package config

import (
	"os"
	"path/filepath"
)

// Config holds Sutra's runtime configuration. Slice 1 covers the daemon's
// listen address and DB path plus the client's host; the bearer token is
// wired in slice 2.
type Config struct {
	ListenAddr  string // daemon bind address, e.g. ":8422"
	DBPath      string // SQLite file path
	Host        string // client target, e.g. "http://localhost:8422"
	Token       string // bearer token (slice 2)
	ProjectsDir string // Claude projects dir transcript ingest/discover is confined to
}

// Load builds a Config from environment variables, falling back to defaults.
//
// The daemon binds to loopback by default: slice 1 is localhost-only, and
// exposing it on the LAN must be an explicit choice (via SUTRA_LISTEN) once
// authentication lands in slice 2.
func Load() Config {
	return Config{
		ListenAddr:  envOr("SUTRA_LISTEN", "127.0.0.1:8422"),
		DBPath:      envOr("SUTRA_DB", defaultDBPath()),
		Host:        envOr("SUTRA_HOST", "http://127.0.0.1:8422"),
		Token:       os.Getenv("SUTRA_TOKEN"),
		ProjectsDir: envOr("SUTRA_PROJECTS_DIR", DefaultProjectsDir()),
	}
}

// DefaultProjectsDir returns the default Claude projects directory
// (~/.claude/projects), falling back to a relative path if the home directory
// cannot be determined.
func DefaultProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".claude", "projects")
	}
	return filepath.Join(home, ".claude", "projects")
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
