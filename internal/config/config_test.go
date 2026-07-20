package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig points XDG_CONFIG_HOME at a temp dir and, if body is non-empty,
// writes it to <tmp>/sutra/config.toml. It returns the temp config home.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	if body != "" {
		dir := filepath.Join(home, "sutra")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	return home
}

func TestLoadDefaultsWhenNoFile(t *testing.T) {
	writeConfig(t, "") // XDG points at an empty temp dir: no config.toml

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:8422" {
		t.Errorf("ListenAddr = %q, want default loopback", cfg.ListenAddr)
	}
	if cfg.Host != "http://127.0.0.1:8422" {
		t.Errorf("Host = %q, want default", cfg.Host)
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty by default", cfg.Token)
	}
	if cfg.DBPath == "" || cfg.ProjectsDir == "" {
		t.Errorf("DBPath/ProjectsDir should have defaults, got %q / %q", cfg.DBPath, cfg.ProjectsDir)
	}
}

func TestLoadFromFile(t *testing.T) {
	writeConfig(t, `
listen_addr  = "0.0.0.0:9000"
host         = "http://192.168.1.10:9000"
token        = "s3cret"
db_path      = "/data/sutra.db"
projects_dir = "/data/projects"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		ListenAddr:  "0.0.0.0:9000",
		Host:        "http://192.168.1.10:9000",
		Token:       "s3cret",
		DBPath:      "/data/sutra.db",
		ProjectsDir: "/data/projects",
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadPartialFileKeepsDefaults(t *testing.T) {
	// Only the token is set; every other field must fall back to its default.
	writeConfig(t, `token = "only-token"`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Token != "only-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "only-token")
	}
	if cfg.ListenAddr != "127.0.0.1:8422" {
		t.Errorf("ListenAddr = %q, want default when unset in file", cfg.ListenAddr)
	}
}

func TestLoadExpandsHomePaths(t *testing.T) {
	writeConfig(t, `
db_path      = "~/.sutra/sutra.db"
projects_dir = "~/work/projects"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if want := filepath.Join(home, ".sutra", "sutra.db"); cfg.DBPath != want {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, want)
	}
	if want := filepath.Join(home, "work", "projects"); cfg.ProjectsDir != want {
		t.Errorf("ProjectsDir = %q, want %q", cfg.ProjectsDir, want)
	}
}

func TestLoadMalformedFileErrors(t *testing.T) {
	writeConfig(t, "listen_addr = = broken")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want an error for malformed TOML")
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	// A typo'd key must fail loudly rather than being silently ignored.
	writeConfig(t, `listne_addr = "127.0.0.1:1"`)

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want an error for an unknown key")
	}
}
