package api

import (
	"context"
	"errors"
	"net/http"
	"time"

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
	// Transcript capture (slice 6).
	mux.HandleFunc("GET /issues/{id}/transcripts", s.listIssueTranscripts)
	mux.HandleFunc("POST /transcripts", s.ingestTranscript)
	mux.HandleFunc("GET /transcripts", s.discoverTranscripts)
	mux.HandleFunc("GET /transcripts/{id}", s.getTranscript)
	mux.HandleFunc("POST /transcripts/{id}/link", s.linkTranscript)
	return mux
}

// Run opens the service and serves HTTP on the configured address. It blocks
// until ctx is cancelled (then it shuts down gracefully) or the server fails.
func Run(ctx context.Context, cfg config.Config) error {
	svc, err := service.New(cfg)
	if err != nil {
		return err
	}
	defer svc.Close()

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: Handler(svc)}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
