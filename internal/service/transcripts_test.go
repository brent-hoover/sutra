package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

// newService returns a Service whose store lives in a temp dir and whose
// transcript ingest/discover is confined to projectsDir.
func newService(t *testing.T, projectsDir string) *service.Service {
	t.Helper()
	svc, err := service.New(config.Config{
		DBPath:      filepath.Join(t.TempDir(), "test.db"),
		ProjectsDir: projectsDir,
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func sessionLines() []string {
	return []string{
		`{"type":"user","message":{"role":"user","content":"Fix the login bug"},"timestamp":"2026-07-16T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I will fix it."}]},"timestamp":"2026-07-16T10:00:05Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{}}]}}`,
	}
}

func writeSession(t *testing.T, dir, sessionID string, lines []string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}

// Finding 1: ingest must be confined to .jsonl files beneath ProjectsDir.
func TestIngestRejectsPathOutsideProjectsDir(t *testing.T) {
	projectsDir := t.TempDir()
	outsideDir := t.TempDir()
	svc := newService(t, projectsDir)

	// A .jsonl file outside the projects dir must be rejected as invalid.
	outside := writeSession(t, outsideDir, "outside-session", sessionLines())
	if _, err := svc.IngestTranscript(outside); !errors.Is(err, domain.ErrInvalidTranscript) {
		t.Fatalf("ingest of path outside ProjectsDir: err = %v, want ErrInvalidTranscript", err)
	}

	// A non-.jsonl file inside the projects dir must also be rejected.
	nonJSONL := filepath.Join(projectsDir, "notes.txt")
	if err := os.WriteFile(nonJSONL, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write non-jsonl: %v", err)
	}
	if _, err := svc.IngestTranscript(nonJSONL); !errors.Is(err, domain.ErrInvalidTranscript) {
		t.Fatalf("ingest of non-.jsonl: err = %v, want ErrInvalidTranscript", err)
	}

	// A .jsonl file inside the projects dir is accepted.
	inside := writeSession(t, filepath.Join(projectsDir, "-Users-me-proj"), "inside-session", sessionLines())
	if _, err := svc.IngestTranscript(inside); err != nil {
		t.Fatalf("ingest of valid path inside ProjectsDir: %v", err)
	}
}

// Finding 7: ingestion is lossless — every line becomes a Message and Raw is the
// exact line, including blank lines.
// Finding 8: a tool_result is classified as RoleTool even when it carries text.
func TestIngestLosslessAndToolResultRole(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hello"}}`,
		``, // blank line must be preserved as its own Message
		`  {"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"tool output text"}]}}`,
	}
	path := writeSession(t, filepath.Join(projectsDir, "-Users-me-proj"), "lossless-session", lines)

	tr, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if len(tr.Messages) != len(lines) {
		t.Fatalf("message count = %d, want %d (blank line dropped?)", len(tr.Messages), len(lines))
	}
	for i, want := range lines {
		if tr.Messages[i].Raw != want {
			t.Fatalf("message %d Raw = %q, want verbatim %q", i, tr.Messages[i].Raw, want)
		}
	}
	if tr.Messages[2].Role != domain.RoleTool {
		t.Fatalf("tool_result-with-text role = %q, want %q", tr.Messages[2].Role, domain.RoleTool)
	}
}

// Findings 5 & 6: re-ingesting a linked transcript preserves the issue link and
// updates messages in place (their ids are preserved, not recreated).
func TestReingestPreservesMessageIDsAndIssueLink(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)

	path := writeSession(t, filepath.Join(projectsDir, "-Users-me-proj"), "reingest-session", sessionLines())

	first, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	issue, err := svc.CreateIssue("Linkable", "issue to link a transcript to")
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if _, err := svc.LinkTranscript(first.ID, issue.ID); err != nil {
		t.Fatalf("link transcript: %v", err)
	}

	// Capture the linked message ids as persisted.
	linked, err := svc.GetTranscript(first.ID)
	if err != nil {
		t.Fatalf("get transcript: %v", err)
	}

	second, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}

	// Finding 5: the returned struct reports the preserved issue link.
	if second.IssueID == nil || *second.IssueID != issue.ID {
		t.Fatalf("re-ingest IssueID = %v, want %q", second.IssueID, issue.ID)
	}

	// Finding 6: messages are updated in place, keeping their ids by seq.
	if len(second.Messages) != len(linked.Messages) {
		t.Fatalf("message count = %d, want %d", len(second.Messages), len(linked.Messages))
	}
	for i := range linked.Messages {
		if second.Messages[i].Seq != linked.Messages[i].Seq {
			t.Fatalf("message %d seq changed", i)
		}
		if second.Messages[i].ID != linked.Messages[i].ID {
			t.Fatalf("message seq %d id = %q, want preserved %q",
				linked.Messages[i].Seq, second.Messages[i].ID, linked.Messages[i].ID)
		}
	}
}

// Finding (iter5 #1): a JSONL line larger than the old 8 MiB scanner cap must
// still be ingested losslessly.
func TestIngestLargeLine(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)

	big := strings.Repeat("x", 10*1024*1024) // 10 MiB, larger than the old cap
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"start"},"timestamp":"2026-07-16T10:00:00Z"}`,
		big,
	}
	path := writeSession(t, filepath.Join(projectsDir, "-Users-me-proj"), "biglinesession", lines)

	tr, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("ingest large line: %v", err)
	}
	if len(tr.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(tr.Messages))
	}
	if len(tr.Messages[1].Raw) != len(big) {
		t.Errorf("large line raw len = %d, want %d", len(tr.Messages[1].Raw), len(big))
	}
}

// Finding (iter5 #2/#3): relinking to the same issue is idempotent (one ledger
// entry); relinking to a different issue is rejected.
func TestRelinkIdempotentAndConflict(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	path := writeSession(t, filepath.Join(projectsDir, "-Users-me-proj"), "relinksession", sessionLines())

	tr, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	a, err := svc.CreateIssue("Issue A", "body")
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := svc.CreateIssue("Issue B", "body")
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	if _, err := svc.LinkTranscript(tr.ID, a.ID); err != nil {
		t.Fatalf("first link: %v", err)
	}
	// Re-link to the same issue: no error, and no second ledger entry.
	if _, err := svc.LinkTranscript(tr.ID, a.ID); err != nil {
		t.Fatalf("idempotent relink: %v", err)
	}
	history, err := svc.IssueHistory(a.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	linked := 0
	for _, e := range history {
		if e.Kind == domain.LedgerLinked {
			linked++
		}
	}
	if linked != 1 {
		t.Errorf("linked ledger entries = %d, want 1 (idempotent)", linked)
	}

	// Re-link to a different issue: rejected.
	if _, err := svc.LinkTranscript(tr.ID, b.ID); !errors.Is(err, domain.ErrInvalidTranscript) {
		t.Errorf("relink to different issue: err = %v, want ErrInvalidTranscript", err)
	}
}

// Re-ingesting a linked transcript must advance the owning issue's updated_at.
// Re-ingesting a linked transcript bumps its issue and records a ledger entry
// ONLY when the session actually changed (newer file mtime). An unchanged
// re-ingest is a no-op, so merely re-reading a session never manufactures
// activity on its issue.
func TestReingestLinkedBumpsIssueOnlyWhenChanged(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	path := writeSession(t, filepath.Join(projectsDir, "-Users-me-proj"), "reingestbump", sessionLines())

	tr, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	iss, err := svc.CreateIssue("Linked", "body")
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if _, err := svc.LinkTranscript(tr.ID, iss.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	before := issueUpdatedAt(t, svc, iss.ID)

	// Unchanged re-ingest: no bump, no ledger update.
	if _, err := svc.IngestTranscript(path); err != nil {
		t.Fatalf("re-ingest unchanged: %v", err)
	}
	if got := issueUpdatedAt(t, svc, iss.ID); got.After(before) {
		t.Errorf("unchanged re-ingest advanced updated_at: %s > %s", got, before)
	}
	if n := countLedger(t, svc, iss.ID, domain.LedgerUpdated); n != 0 {
		t.Errorf("unchanged re-ingest wrote %d ledger updates, want 0", n)
	}

	// Changed session (newer mtime): bump + one ledger update.
	newer := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, newer, newer); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if _, err := svc.IngestTranscript(path); err != nil {
		t.Fatalf("re-ingest changed: %v", err)
	}
	if got := issueUpdatedAt(t, svc, iss.ID); !got.After(before) {
		t.Errorf("changed re-ingest did not advance updated_at: %s !> %s", got, before)
	}
	if n := countLedger(t, svc, iss.ID, domain.LedgerUpdated); n != 1 {
		t.Errorf("changed re-ingest wrote %d ledger updates, want 1", n)
	}
}
