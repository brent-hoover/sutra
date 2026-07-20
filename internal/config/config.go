package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// Open once and derive both the permission check and the decode from the same
	// handle, so a concurrent replacement can't make us check one file and read
	// the token from another (TOCTOU).
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}

	var fc fileConfig
	md, err := toml.NewDecoder(f).Decode(&fc)
	if err != nil {
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("unknown key(s) in config %s: %v", path, undecoded)
	}

	// A plaintext token must not be readable by other users. Like ssh and
	// postgres, refuse to start when the file holding it is group/world-accessible.
	if fc.Token != "" {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return Config{}, fmt.Errorf("config %s has insecure permissions %#o: it contains a token, so it must be owner-only (chmod 600)", path, perm)
		}
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

	// TOML has no shell expansion, so a home-relative path like "~/.sutra/sutra.db"
	// would otherwise be created under the current directory. Expand it for the
	// filesystem-path fields.
	cfg.DBPath = expandHome(cfg.DBPath)
	cfg.ProjectsDir = expandHome(cfg.ProjectsDir)
	return cfg, nil
}

// expandHome replaces a leading "~/" (or a bare "~") with the user's home
// directory. It returns the path unchanged if there is no "~" prefix or the
// home directory cannot be determined.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
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
