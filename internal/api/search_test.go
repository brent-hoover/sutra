package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

func newSearchServer(t *testing.T) (*httptest.Server, *service.Service) {
	t.Helper()
	dir := t.TempDir()
	svc, err := service.New(config.Config{
		DBPath:      filepath.Join(dir, "t.db"),
		ProjectsDir: filepath.Join(dir, "projects"),
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	srv := httptest.NewServer(api.Handler(svc))
	t.Cleanup(srv.Close)
	return srv, svc
}

func searchStatus(t *testing.T, srv *httptest.Server, q url.Values) (int, []byte) {
	t.Helper()
	resp, err := http.Get(srv.URL + "/search?" + q.Encode())
	if err != nil {
		t.Fatalf("GET /search: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body
}

// TestSearchAPIValidation: a missing q is 400, an invalid kind scope is 400.
func TestSearchAPIValidation(t *testing.T) {
	srv, _ := newSearchServer(t)

	if got, _ := searchStatus(t, srv, url.Values{}); got != http.StatusBadRequest {
		t.Errorf("missing q status = %d, want 400", got)
	}
	if got, _ := searchStatus(t, srv, url.Values{"q": {"x"}, "kind": {"bogus"}}); got != http.StatusBadRequest {
		t.Errorf("invalid kind status = %d, want 400", got)
	}
	if got, _ := searchStatus(t, srv, url.Values{"q": {"x"}, "limit": {"-1"}}); got != http.StatusBadRequest {
		t.Errorf("negative limit status = %d, want 400", got)
	}
	if got, _ := searchStatus(t, srv, url.Values{"q": {"x"}, "limit": {"notanint"}}); got != http.StatusBadRequest {
		t.Errorf("non-integer limit status = %d, want 400", got)
	}
}

// TestSearchAPIHappyPath: a valid query returns 200 and a structured result.
func TestSearchAPIHappyPath(t *testing.T) {
	srv, svc := newSearchServer(t)
	const term = "borogoves"
	if _, err := svc.CreateIssue("subject", "body with "+term); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	status, body := searchStatus(t, srv, url.Values{"q": {term}})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", status, body)
	}
	var res domain.SearchResults
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Query != term {
		t.Errorf("query = %q, want %q", res.Query, term)
	}
	if len(res.Hits) != 1 || res.Hits[0].Kind != domain.KindIssue {
		t.Errorf("expected one issue hit, got %+v", res.Hits)
	}
}
