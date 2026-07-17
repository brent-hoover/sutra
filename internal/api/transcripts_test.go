package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/service"
)

// TestLinkTranscriptConflict verifies that relinking a transcript to a
// different issue is reported to clients as HTTP 409, not 500.
func TestLinkTranscriptConflict(t *testing.T) {
	dir := t.TempDir()
	projects := filepath.Join(dir, "projects")
	sessDir := filepath.Join(projects, "-Users-me-proj")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sessionPath := filepath.Join(sessDir, "11111111-2222-3333-4444-555555555555.jsonl")
	line := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-07-16T10:00:00Z"}` + "\n"
	if err := os.WriteFile(sessionPath, []byte(line), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	svc, err := service.New(config.Config{
		DBPath:      filepath.Join(dir, "t.db"),
		ProjectsDir: projects,
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(func() { svc.Close() })

	a, err := svc.CreateIssue("Issue A", "body")
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := svc.CreateIssue("Issue B", "body")
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	tr, err := svc.IngestTranscript(sessionPath)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	srv := httptest.NewServer(api.Handler(svc))
	t.Cleanup(srv.Close)

	link := func(issueID string) int {
		body := strings.NewReader(`{"issue_id":"` + issueID + `"}`)
		resp, err := http.Post(srv.URL+"/transcripts/"+tr.ID+"/link", "application/json", body)
		if err != nil {
			t.Fatalf("POST link: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := link(a.ID); got != http.StatusOK {
		t.Fatalf("first link status = %d, want 200", got)
	}
	if got := link(b.ID); got != http.StatusConflict {
		t.Errorf("relink to different issue status = %d, want 409", got)
	}
}
