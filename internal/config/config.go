package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds Sutra's runtime configuration: the daemon's listen address and
// DB path, the client's target host, the bearer token, and the Claude projects
// directory transcript ingest/discover is confined to.
type Config struct {
	ListenAddr  string // daemon bind address, e.g. ":8422"
	DBPath      string // SQLite file path
	Host        string // client target, e.g. "http://localhost:8422"
	Token       string // bearer token (empty disables auth on loopback)
	ProjectsDir string // Claude projects dir transcript ingest/discover is confined to
}

// fileConfig mirrors the on-disk TOML. Fields left unset in the file stay empty
// here and fall back to defaults() in Load.
type fileConfig struct {
	ListenAddr  string `toml:"listen_addr"`
	DBPath      string `toml:"db_path"`
	Host        string `toml:"host"`
	Token       string `toml:"token"`
	ProjectsDir string `toml:"projects_dir"`
}

// Load builds a Config from the TOML file at ConfigPath, overlaying any set
// keys onto the defaults. A missing file is not an error (defaults are used); a
// malformed file or an unknown key is, so misconfiguration fails loudly.
//
// The daemon binds to loopback by default: exposing it on the LAN must be an
// explicit choice (listen_addr) and requires a token.
func Load() (Config, error) {
	cfg := defaults()

	path := ConfigPath()
	var fc fileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("unknown key(s) in config %s: %v", path, undecoded)
	}

	if fc.ListenAddr != "" {
		cfg.ListenAddr = fc.ListenAddr
	}
	if fc.DBPath != "" {
		cfg.DBPath = fc.DBPath
	}
	if fc.Host != "" {
		cfg.Host = fc.Host
	}
	if fc.Token != "" {
		cfg.Token = fc.Token
	}
	if fc.ProjectsDir != "" {
		cfg.ProjectsDir = fc.ProjectsDir
	}
	return cfg, nil
}

// defaults returns the configuration used when no file (or no key) overrides it.
func defaults() Config {
	return Config{
		ListenAddr:  "127.0.0.1:8422",
		DBPath:      defaultDBPath(),
		Host:        "http://127.0.0.1:8422",
		Token:       "",
		ProjectsDir: DefaultProjectsDir(),
	}
}

// ConfigPath returns the path to the TOML config file:
// $XDG_CONFIG_HOME/sutra/config.toml, defaulting to ~/.config/sutra/config.toml.
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "sutra", "config.toml")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "sutra", "config.toml")
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

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "sutra.db"
	}
	return filepath.Join(home, ".sutra", "sutra.db")
}
