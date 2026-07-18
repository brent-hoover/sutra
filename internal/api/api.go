package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/service"
)

// isLoopbackAddr reports whether a listen address binds only the loopback
// interface. An empty host (e.g. ":8422") binds all interfaces and is not
// loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

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
	mux.HandleFunc("GET /issues/{id}/comments", s.listComments)
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
	// Linking & labels (slice 4).
	mux.HandleFunc("PUT /issues/{id}/parent", s.setParent)
	mux.HandleFunc("POST /issues/{id}/relations", s.relateIssue)
	mux.HandleFunc("POST /issues/{id}/blocks", s.blockIssue)
	mux.HandleFunc("POST /issues/{id}/labels", s.addLabel)
	mux.HandleFunc("DELETE /issues/{id}/labels", s.removeLabel)
	// Search (slice 7).
	mux.HandleFunc("GET /search", s.search)
	return mux
}

// HandlerWithAuth builds the routes and, when token is non-empty, wraps them so
// every request must present "Authorization: Bearer <token>".
func HandlerWithAuth(svc *service.Service, token string) http.Handler {
	return authMiddleware(token, Handler(svc))
}

// authMiddleware enforces bearer-token auth when token is non-empty. An empty
// token (none configured) is a no-op, keeping localhost use friction-free; the
// LAN deployment sets SUTRA_TOKEN to require it.
func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	// Hash both sides to fixed-size digests before the constant-time compare, so
	// the comparison length (and thus the configured token length) never leaks
	// through timing.
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Scheme is case-insensitive per RFC 7235; only the token is secret, so
		// only it gets a constant-time compare. Fields tolerates arbitrary
		// whitespace between scheme and credentials and requires exactly two.
		fields := strings.Fields(r.Header.Get("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		got := sha256.Sum256([]byte(fields[1]))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Run opens the service and serves HTTP on the configured address. It blocks
// until ctx is cancelled (then it shuts down gracefully) or the server fails.
func Run(ctx context.Context, cfg config.Config) error {
	// Never expose an unauthenticated daemon beyond loopback: require a token
	// when binding to a non-loopback (LAN/all-interfaces) address.
	if cfg.Token == "" && !isLoopbackAddr(cfg.ListenAddr) {
		return fmt.Errorf("refusing to serve on non-loopback address %q without SUTRA_TOKEN set", cfg.ListenAddr)
	}

	svc, err := service.New(cfg)
	if err != nil {
		return err
	}
	defer svc.Close()

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: HandlerWithAuth(svc, cfg.Token),
		// Bound how long an unauthenticated peer may hold a connection before its
		// request headers arrive, mitigating slow-header (Slowloris) DoS on the
		// LAN-facing daemon. IdleTimeout caps kept-alive idle connections.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
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
