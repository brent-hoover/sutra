package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// TestReconcileSearch verifies that the one-time backfill restores a partially
// populated index across all three entity kinds without duplicating rows that
// are already indexed, and that it does not run again once its marker is set.
func TestReconcileSearch(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	const term = "zqxbackfill"
	now := time.Now().UTC()

	iss := domain.Issue{
		ID: domain.NewID(), Subject: "backfill subject", Body: "body has " + term,
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(iss, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated,
	}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if err := s.CreateDocument(domain.Document{
		ID: domain.NewID(), IssueID: iss.ID, Kind: domain.DocPlan,
		Content: "doc content with " + term, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create document: %v", err)
	}
	if _, err := s.UpsertTranscript(domain.Transcript{
		ID: domain.NewID(), SessionID: "backfill-session", SourcePath: "/x.jsonl",
		CapturedAt: now, CreatedAt: now,
		Messages: []domain.Message{{Seq: 0, Role: domain.RoleUser, Text: "message has " + term, Raw: "{}"}},
	}); err != nil {
		t.Fatalf("upsert transcript: %v", err)
	}

	// Simulate an interrupted first migration that left the index PARTIALLY
	// populated (triggers indexed the issue) but never set the marker: clear the
	// marker and drop only the document + message index rows.
	if _, err := s.db.Exec(`DELETE FROM schema_meta WHERE key = ?`, searchIndexedKey); err != nil {
		t.Fatalf("clear marker: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM search_fts WHERE kind IN ('document','message')`); err != nil {
		t.Fatalf("partial wipe: %v", err)
	}
	if hits := searchTerm(t, s, term); len(hits) != 1 || hits[0].Kind != domain.KindIssue {
		t.Fatalf("expected only the issue indexed after partial wipe, got %d hits", len(hits))
	}

	// Reconcile (as migrate would on the next open) must restore the missing
	// kinds without duplicating the issue that was still indexed.
	if err := s.reconcileSearch(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	counts := map[domain.SearchKind]int{}
	for _, h := range searchTerm(t, s, term) {
		counts[h.Kind]++
	}
	if counts[domain.KindIssue] != 1 || counts[domain.KindDocument] != 1 || counts[domain.KindMessage] != 1 {
		t.Fatalf("reconcile did not restore all kinds exactly once, got %v", counts)
	}

	// The marker is now set, so a second reconcile is a no-op — no duplicates and
	// no re-scan.
	if err := s.reconcileSearch(); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if hits := searchTerm(t, s, term); len(hits) != 3 {
		t.Fatalf("expected 3 hits after idempotent reconcile, got %d", len(hits))
	}
}

func searchTerm(t *testing.T, s *Store, term string) []domain.SearchHit {
	t.Helper()
	hits, err := s.Search(domain.SearchQuery{Text: term})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	return hits
}
