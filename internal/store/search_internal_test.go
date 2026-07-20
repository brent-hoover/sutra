package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// TestReconcileSearch verifies that the one-time rebuild restores an index left
// in an inconsistent state by an interrupted migration — missing rows added,
// stale/orphaned/duplicate rows removed — so it exactly mirrors the source
// tables, and that it does not run again once its marker is set.
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
	}, ""); err != nil {
		t.Fatalf("upsert transcript: %v", err)
	}

	// Simulate a DB upgraded from the prior (v1) additive-backfill version whose
	// index it may have left inconsistent: the OLD marker is present, the current
	// versioned marker is absent, and the index has a missing row (drop the
	// message), a stale/orphaned row (an FTS entry for a nonexistent issue id),
	// and a duplicate row (a second FTS entry for the real issue).
	if _, err := s.db.Exec(`DELETE FROM schema_meta WHERE key = ?`, searchIndexedKey); err != nil {
		t.Fatalf("clear current marker: %v", err)
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('search_indexed', 'v1')`); err != nil {
		t.Fatalf("set old marker: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM search_fts WHERE kind = 'message'`); err != nil {
		t.Fatalf("drop message row: %v", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO search_fts(kind, ref_id, text) VALUES ('issue', 'ghost-id', ?)`, "orphan "+term); err != nil {
		t.Fatalf("insert orphan: %v", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO search_fts(kind, ref_id, text) VALUES ('issue', ?, ?)`, iss.ID, "dup "+term); err != nil {
		t.Fatalf("insert duplicate: %v", err)
	}

	// Reconcile (as migrate would on the next open) must rebuild the index to
	// exactly mirror the source: restore the message, drop the orphan and the
	// duplicate. (A surviving orphan would also make hydration fail with
	// ErrNotFound, so a clean search proves it was removed.)
	if err := s.reconcileSearch(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	counts := map[domain.SearchKind]int{}
	for _, h := range searchTerm(t, s, term) {
		counts[h.Kind]++
	}
	if counts[domain.KindIssue] != 1 || counts[domain.KindDocument] != 1 || counts[domain.KindMessage] != 1 {
		t.Fatalf("reconcile did not rebuild to mirror source exactly once, got %v", counts)
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
