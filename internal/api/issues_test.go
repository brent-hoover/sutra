package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	svc, err := service.New(config.Config{DBPath: filepath.Join(dir, "t.db"), ProjectsDir: filepath.Join(dir, "projects")})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	srv := httptest.NewServer(api.Handler(svc))
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateIssueBodyDecoding(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid", `{"subject":"a","body":"b"}`, http.StatusCreated},
		{"trailing whitespace ok", "{\"subject\":\"a\",\"body\":\"b\"}\n  \t", http.StatusCreated},
		{"concatenated json rejected", `{"subject":"a","body":"b"}{"subject":"c","body":"d"}`, http.StatusBadRequest},
		{"malformed trailing rejected", `{"subject":"a","body":"b"} garbage`, http.StatusBadRequest},
		{"invalid json rejected", `{`, http.StatusBadRequest},
		{"missing body rejected", `{"subject":"a"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/issues", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func postIssue(t *testing.T, srv *httptest.Server, body string) domain.Issue {
	t.Helper()
	resp, err := http.Post(srv.URL+"/issues", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /issues: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /issues status = %d: %s", resp.StatusCode, raw)
	}
	var issue domain.Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	return issue
}

func TestCreateChildIssue(t *testing.T) {
	srv := newTestServer(t)
	parent := postIssue(t, srv, `{"subject":"parent","body":"b"}`)

	child := postIssue(t, srv, `{"subject":"child","body":"b","parent_id":"`+parent.ID+`"}`)
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("child parent_id = %v, want %q", child.ParentID, parent.ID)
	}

	// A nonexistent parent is a client error, not a persisted orphan.
	resp, err := http.Post(srv.URL+"/issues", "application/json", strings.NewReader(`{"subject":"c","body":"b","parent_id":"nope"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad parent status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestListCommentsUnknownIssue(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/issues/does-not-exist/comments")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
