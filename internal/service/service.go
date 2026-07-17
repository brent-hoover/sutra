package service

import (
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/store"
)

// Service implements Sutra's use-cases over the store.
type Service struct {
	store *store.Store
}

// New opens the store at the configured path and returns a Service.
func New(cfg config.Config) (*Service, error) {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return &Service{store: st}, nil
}

// Close releases the underlying store.
func (s *Service) Close() error { return s.store.Close() }
