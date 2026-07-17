package api

import (
	"net/http"

	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/service"
)

// Server holds the dependencies the HTTP handlers need.
type Server struct {
	svc *service.Service
}

// Handler builds the HTTP routes over the given service.
func Handler(svc *service.Service) http.Handler {
	s := &Server{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /issues", s.createIssue)
	mux.HandleFunc("GET /issues/{id}", s.getIssue)
	return mux
}

// Run opens the service and serves HTTP on the configured address. Blocking.
func Run(cfg config.Config) error {
	svc, err := service.New(cfg)
	if err != nil {
		return err
	}
	defer svc.Close()
	return http.ListenAndServe(cfg.ListenAddr, Handler(svc))
}
