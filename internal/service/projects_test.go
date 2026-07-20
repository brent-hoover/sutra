package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brent-hoover/sutra/internal/domain"
)

// A transcript nested below the encoded-cwd folder (e.g. a subagent session)
// still auto-associates to the project, because association uses the top-level
// path component under the projects dir, not the .jsonl's immediate parent.
func TestIngestNestedTranscriptAssociatesToProject(t *testing.T) {
	projects := t.TempDir()
	svc := newService(t, projects)

	repo := "/repo/deep"
	p, err := svc.CreateProject("Deep", repo, "", "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	// <encoded-cwd>/<session>/subagents/<agent>.jsonl
	dir := filepath.Join(projects, domain.EncodeCWD(repo), "sess1", "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "agent0000-0000-0000-0000-000000000001.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	tr, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if tr.ProjectID == nil || *tr.ProjectID != p.ID {
		t.Errorf("nested transcript project = %v, want %s", tr.ProjectID, p.ID)
	}
}
