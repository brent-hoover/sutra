package store_test

import (
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/store"
)

// TestGetIssueViewConsistentUnderConcurrentWrites exercises the transactional
// snapshot read in GetIssueView against a *second* store handle writing to the
// same database file concurrently. Every read must succeed and return a
// well-formed projection: the base label is always present, and the toggled
// label is either fully present or fully absent — never a torn state — proving
// the view reads a single consistent snapshot rather than interleaving with the
// concurrent writer's commits.
//
// Note: a fully deterministic interleaving (forcing a commit between the view's
// issue read and its dependent collection reads) would require a test hook
// inside GetIssueView. That is deliberately not added — the snapshot is enforced
// by SQLite's WAL read transaction, and this contention test is the regression
// guard for it.
func TestGetIssueViewConsistentUnderConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.db")
	reader, err := store.Open(path)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()

	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "s", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := reader.CreateIssue(iss, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated,
	}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if err := reader.AddLabel(iss.ID, "base", domain.LedgerEntry{
		ID: domain.NewID(), IssueID: iss.ID, Kind: domain.LedgerUpdated, Field: "label", NewValue: "base",
	}); err != nil {
		t.Fatalf("add base label: %v", err)
	}

	// A separate handle to the same file drives concurrent writes.
	writer, err := store.Open(path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer writer.Close()

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() { // writer: toggle a label on and off
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = writer.AddLabel(iss.ID, "toggle", domain.LedgerEntry{
				ID: domain.NewID(), IssueID: iss.ID, Kind: domain.LedgerUpdated, Field: "label", NewValue: "toggle",
			})
			_ = writer.RemoveLabel(iss.ID, "toggle", domain.LedgerEntry{
				ID: domain.NewID(), IssueID: iss.ID, Kind: domain.LedgerUpdated, Field: "label", OldValue: "toggle",
			})
		}
	}()

	go func() { // reader: assert every view is a consistent snapshot
		defer wg.Done()
		for i := 0; i < iterations*4; i++ {
			view, err := reader.GetIssueView(iss.ID)
			if err != nil {
				t.Errorf("GetIssueView: %v", err)
				return
			}
			labels := append([]string(nil), view.Labels...)
			sort.Strings(labels)
			switch {
			case len(labels) == 1 && labels[0] == "base":
			case len(labels) == 2 && labels[0] == "base" && labels[1] == "toggle":
			default:
				t.Errorf("torn label snapshot: %v", labels)
				return
			}
		}
	}()

	wg.Wait()
}
