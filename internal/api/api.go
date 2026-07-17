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
	mux.HandleFunc("GET /issues", s.listIssues)
	mux.HandleFunc("GET /issues/{id}", s.getIssue)
	mux.HandleFunc("PATCH /issues/{id}", s.updateIssue)
	mux.HandleFunc("DELETE /issues/{id}", s.deleteIssue)
	mux.HandleFunc("GET /issues/{id}/history", s.issueHistory)
	mux.HandleFunc("POST /issues/{id}/comments", s.createComment)
	mux.HandleFunc("POST /issues/{id}/documents", s.attachDocument)
	mux.HandleFunc("GET /issues/{id}/documents", s.listDocuments)
	mux.HandleFunc("GET /documents/{id}", s.getDocument)
	mux.HandleFunc("PATCH /documents/{id}", s.updateDocument)
	mux.HandleFunc("DELETE /documents/{id}", s.deleteDocument)
	// Transcript capture (slice 6).
	mux.HandleFunc("GET /issues/{id}/transcripts", s.listIssueTranscripts)
	mux.HandleFunc("POST /transcripts", s.ingestTranscript)
	mux.HandleFunc("GET /transcripts", s.discoverTranscripts)
	mux.HandleFunc("GET /transcripts/{id}", s.getTranscript)
	mux.HandleFunc("POST /transcripts/{id}/link", s.linkTranscript)
	// Search (slice 7).
	mux.HandleFunc("GET /search", s.search)
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
