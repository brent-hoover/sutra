package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/service"
)

// authServer starts a daemon whose Handler enforces the given bearer token.
func authServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	svc, err := service.New(config.Config{DBPath: filepath.Join(dir, "t.db"), ProjectsDir: filepath.Join(dir, "projects")})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	srv := httptest.NewServer(api.HandlerWithAuth(svc, token))
	t.Cleanup(srv.Close)
	return srv
}

func getWithAuth(t *testing.T, url, authHeader string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestBearerAuth(t *testing.T) {
	srv := authServer(t, "s3cret")

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"correct token", "Bearer s3cret", http.StatusOK},
		{"lowercase scheme", "bearer s3cret", http.StatusOK},
		{"multiple spaces", "Bearer  s3cret", http.StatusOK},
		{"missing token", "", http.StatusUnauthorized},
		{"scheme only", "Bearer", http.StatusUnauthorized},
		{"extra credential field", "Bearer s3cret extra", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"malformed header", "s3cret", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getWithAuth(t, srv.URL+"/issues", tc.header); got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// With no token configured, auth is disabled (localhost default).
func TestNoTokenDisablesAuth(t *testing.T) {
	srv := authServer(t, "")
	if got := getWithAuth(t, srv.URL+"/issues", ""); got != http.StatusOK {
		t.Errorf("status = %d, want 200 (auth disabled)", got)
	}
}

// Without a token, state-changing requests must carry X-Sutra-Client (CSRF
// guard); safe methods are unaffected.
func TestCSRFGuardWithoutToken(t *testing.T) {
	srv := authServer(t, "")

	do := func(method, path string, header map[string]string) int {
		req, err := http.NewRequest(method, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		for k, v := range header {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// POST without the header → 403 (simulates a cross-origin simple request).
	if got := do(http.MethodPost, "/issues", nil); got != http.StatusForbidden {
		t.Errorf("POST without X-Sutra-Client = %d, want 403", got)
	}
	// POST with the header passes the guard (reaches the handler; 400 for the
	// empty body, not 403).
	if got := do(http.MethodPost, "/issues", map[string]string{"X-Sutra-Client": "cli"}); got == http.StatusForbidden {
		t.Errorf("POST with X-Sutra-Client = 403, want it to pass the CSRF guard")
	}
	// Safe method is never blocked.
	if got := do(http.MethodGet, "/issues", nil); got != http.StatusOK {
		t.Errorf("GET = %d, want 200 (safe method, no CSRF guard)", got)
	}
}
