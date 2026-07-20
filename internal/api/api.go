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
	// Activity feed (slice 9). POST, not GET: building the feed auto-ingests
	// recent sessions (a persistent write), so the endpoint is state-changing and
	// must not be reachable by safe-method prefetchers/crawlers.
	mux.HandleFunc("POST /activity", s.activity)
	// Projects (slice 10).
	mux.HandleFunc("POST /projects", s.createProject)
	mux.HandleFunc("GET /projects", s.listProjects)
	mux.HandleFunc("GET /projects/{id}", s.getProject)
	mux.HandleFunc("PATCH /projects/{id}", s.updateProject)
	mux.HandleFunc("DELETE /projects/{id}", s.deleteProject)
	// Threads (slice 11).
	mux.HandleFunc("POST /threads", s.createThread)
	mux.HandleFunc("GET /threads", s.listThreads)
	mux.HandleFunc("GET /threads/{id}", s.getThread)
	mux.HandleFunc("PATCH /threads/{id}", s.updateThread)
	mux.HandleFunc("DELETE /threads/{id}", s.deleteThread)
	mux.HandleFunc("POST /threads/{id}/items", s.addThreadItem)
	mux.HandleFunc("DELETE /threads/{id}/items", s.removeThreadItem)
	// Skills (slice 12).
	mux.HandleFunc("POST /skills", s.createSkill)
	mux.HandleFunc("GET /skills", s.listSkills)
	mux.HandleFunc("GET /skills/{id}", s.getSkill)
	mux.HandleFunc("PATCH /skills/{id}", s.updateSkill)
	mux.HandleFunc("DELETE /skills/{id}", s.deleteSkill)
	return mux
}

// HandlerWithAuth builds the routes and, when token is non-empty, wraps them so
// every request must present "Authorization: Bearer <token>". When no token is
// configured (loopback default), it instead guards state-changing methods
// against CSRF, since there is no auth header to make cross-origin forgery fail.
func HandlerWithAuth(svc *service.Service, token string) http.Handler {
	if token == "" {
		return csrfGuard(Handler(svc))
	}
	return authMiddleware(token, Handler(svc))
}

// csrfGuard protects the unauthenticated (tokenless, loopback-only) daemon.
//
// It rejects any request whose Host is not a loopback literal: the tokenless
// daemon only binds loopback, so a non-loopback Host means a DNS-rebinding
// attack (an attacker page rebound to 127.0.0.1 still carries its own Host).
//
// It also requires the X-Sutra-Client header on state-changing methods: a
// cross-origin browser cannot set a custom header on a "simple" request without
// a CORS preflight the daemon never approves, blocking classic CSRF.
//
// When a token is set, the Authorization header provides both guarantees, so
// this guard is not applied.
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackAddr(r.Host) {
			writeError(w, http.StatusForbidden, "unexpected Host header")
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if r.Header.Get("X-Sutra-Client") == "" {
				writeError(w, http.StatusForbidden, "missing X-Sutra-Client header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware enforces bearer-token auth when token is non-empty. An empty
// token (none configured) is a no-op, keeping localhost use friction-free; the
// LAN deployment sets `token` in config.toml to require it.
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
		return fmt.Errorf("refusing to serve on non-loopback address %q without a token set in config.toml", cfg.ListenAddr)
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
