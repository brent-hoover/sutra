package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

func ledgerCount(t *testing.T, s *Store, issueID string, kind domain.LedgerKind) int {
	t.Helper()
	entries, err := s.LedgerFor(issueID)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// A row migrated in before source_mtime existed (empty source_mtime) must have
// it backfilled on the next ingest — without recording ledger activity when the
// content is unchanged — so a recently-modified long-running session appears in
// the activity feed rather than falling back to its old captured_at.
func TestUpsertBackfillsSourceMtimeForMigratedRow(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "linked", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(iss, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated,
	}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	msgs := []domain.Message{{Seq: 0, Role: domain.RoleUser, Text: "hi", Raw: "line0"}}
	tr, err := s.UpsertTranscript(domain.Transcript{
		ID: domain.NewID(), SessionID: "sess-mig", SourcePath: "/x/sess-mig.jsonl",
		CapturedAt: now.Add(-72 * time.Hour), CreatedAt: now, SourceMtime: now.Add(-time.Hour),
		Messages: msgs,
	}, "")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := s.LinkTranscript(tr.ID, iss.ID, domain.LedgerEntry{
		ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerLinked,
	}); err != nil {
		t.Fatalf("link: %v", err)
	}

	// Simulate a pre-source_mtime row.
	if _, err := s.db.Exec(`UPDATE transcripts SET source_mtime = '' WHERE id = ?`, tr.ID); err != nil {
		t.Fatalf("blank source_mtime: %v", err)
	}

	// Re-ingest the SAME content with a fresh mtime (as auto-ingest would).
	fresh := now.Add(-30 * time.Minute)
	got, err := s.UpsertTranscript(domain.Transcript{
		ID: domain.NewID(), SessionID: "sess-mig", SourcePath: "/x/sess-mig.jsonl",
		CapturedAt: now.Add(-72 * time.Hour), CreatedAt: now, SourceMtime: fresh,
		Messages: msgs,
	}, "")
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if !got.SourceMtime.Equal(fresh) {
		t.Errorf("source_mtime not backfilled: got %v, want %v", got.SourceMtime, fresh)
	}
	if n := ledgerCount(t, s, iss.ID, domain.LedgerUpdated); n != 0 {
		t.Errorf("unchanged backfill wrote %d ledger updates, want 0", n)
	}
}
