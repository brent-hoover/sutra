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
		{"missing token", "", http.StatusUnauthorized},
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
