package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/store"
)

// Service implements Sutra's use-cases over the store.
type Service struct {
	store        *store.Store
	projectsDir  string   // absolute projects dir ingest/discover is confined to
	projectsRoot *os.Root // pinned handle to projectsDir; nil if it does not exist
}

// New opens the store at the configured path and returns a Service.
func New(cfg config.Config) (*Service, error) {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	pd := cfg.ProjectsDir
	if pd == "" {
		pd = config.DefaultProjectsDir()
	}
	abs, err := filepath.Abs(pd)
	if err != nil {
		st.Close()
		return nil, err
	}
	// Create the projects directory if absent, then pin it as a directory handle
	// so later ingest and discover operate on this exact directory — a symlink
	// swap of the path afterwards cannot redirect them (TOCTOU-safe). Creating it
	// at startup means sessions added later are discoverable without a restart.
	if err := os.MkdirAll(abs, 0o700); err != nil {
		st.Close()
		return nil, fmt.Errorf("create projects dir: %w", err)
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("open projects dir: %w", err)
	}
	return &Service{store: st, projectsDir: abs, projectsRoot: root}, nil
}

// Close releases the pinned projects root and the underlying store.
func (s *Service) Close() error {
	if s.projectsRoot != nil {
		_ = s.projectsRoot.Close()
	}
	return s.store.Close()
}
