package service

import (
	"errors"
	"fmt"
	"io/fs"
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
	// Create and open the projects dir through a pinned parent handle so its
	// final path component cannot be swapped for a symlink between creation and
	// open (TOCTOU). MkdirAll(parent) is fine directly; the sensitive base is
	// created and opened via the parent root, which refuses symlink traversal.
	// Pinning at startup also means sessions added later need no daemon restart.
	parent, base := filepath.Dir(abs), filepath.Base(abs)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		st.Close()
		return nil, fmt.Errorf("create projects parent: %w", err)
	}
	proot, err := os.OpenRoot(parent)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("open projects parent: %w", err)
	}
	defer proot.Close()
	if err := proot.Mkdir(base, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		st.Close()
		return nil, fmt.Errorf("create projects dir: %w", err)
	}
	root, err := proot.OpenRoot(base)
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
