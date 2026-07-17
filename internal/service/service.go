package service

import (
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/store"
)

// Service implements Sutra's use-cases over the store.
type Service struct {
	store       *store.Store
	projectsDir string // transcript ingest/discover is confined to this directory
}

// New opens the store at the configured path and returns a Service.
func New(cfg config.Config) (*Service, error) {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	projectsDir := cfg.ProjectsDir
	if projectsDir == "" {
		projectsDir = config.DefaultProjectsDir()
	}
	return &Service{store: st, projectsDir: projectsDir}, nil
}

// Close releases the underlying store.
func (s *Service) Close() error { return s.store.Close() }
