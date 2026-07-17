package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// TestBackfillSearch verifies that content written before the FTS index existed
// (simulated by wiping the index) is fully re-indexed across all three entity
// kinds when backfill runs on the next open — guarding against a partial
// backfill that would permanently omit some kinds.
func TestBackfillSearch(t *testing.T) {
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

	// Simulate a pre-search database: rows exist but the index is empty.
	if _, err := s.db.Exec(`DELETE FROM search_fts`); err != nil {
		t.Fatalf("wipe index: %v", err)
	}
	if hits, err := s.Search(domain.SearchQuery{Text: term}); err != nil {
		t.Fatalf("search (empty index): %v", err)
	} else if len(hits) != 0 {
		t.Fatalf("expected 0 hits with empty index, got %d", len(hits))
	}

	// Backfill (as migrate would on the next open) must re-index all three kinds.
	if err := s.backfillSearch(); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	hits, err := s.Search(domain.SearchQuery{Text: term})
	if err != nil {
		t.Fatalf("search (after backfill): %v", err)
	}
	counts := map[domain.SearchKind]int{}
	for _, h := range hits {
		counts[h.Kind]++
	}
	if counts[domain.KindIssue] != 1 || counts[domain.KindDocument] != 1 || counts[domain.KindMessage] != 1 {
		t.Fatalf("backfill did not re-index all kinds, got %v", counts)
	}

	// A second backfill is a no-op (index non-empty) — no duplicates.
	if err := s.backfillSearch(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if hits, err := s.Search(domain.SearchQuery{Text: term}); err != nil {
		t.Fatalf("search (after second backfill): %v", err)
	} else if len(hits) != 3 {
		t.Fatalf("expected 3 hits after idempotent backfill, got %d", len(hits))
	}
}
