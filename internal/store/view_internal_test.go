package store

// White-box test: it reaches into the unexported db handle to install an atomic
// cross-collection invariant that a *black-box* test cannot, so it can prove the
// GetIssueView snapshot spans separate queries.

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// TestGetIssueViewSnapshotSpansQueries proves GetIssueView reads the issue row
// and its derived collections from one consistent snapshot, not query-by-query.
//
// A writer on a second handle atomically flips two pieces of state that
// GetIssueView reads with *different* queries: the issue row's owner and the
// presence of a label. The invariant owner=="on" IFF the "toggle" label is
// present holds in every committed DB state. A non-transactional view — reading
// the issue row, then labels, in separate statements — could observe the row
// from one commit and the labels from another and see the invariant violated;
// the transactional snapshot cannot. Over many iterations against a concurrent
// writer this deterministically fails if the surrounding transaction in
// GetIssueView is removed, and passes with it in place.
func TestGetIssueViewSnapshotSpansQueries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.db")
	reader, err := Open(path)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()

	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "s", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		Owner: "off", CreatedAt: now, UpdatedAt: now,
	}
	if err := reader.CreateIssue(iss, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated,
	}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	writer, err := Open(path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer writer.Close()

	// flip atomically sets owner and the label's presence together, so any
	// committed state satisfies owner=="on" IFF the label is present.
	flip := func(on bool) error {
		tx, err := writer.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		owner := "off"
		if on {
			owner = "on"
		}
		if _, err := tx.Exec(`UPDATE issues SET owner = ? WHERE id = ?`, owner, iss.ID); err != nil {
			return err
		}
		if on {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO issue_label (issue_id, label) VALUES (?, 'toggle')`, iss.ID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(`DELETE FROM issue_label WHERE issue_id = ? AND label = 'toggle'`, iss.ID); err != nil {
				return err
			}
		}
		return tx.Commit()
	}

	const iterations = 300
	var wg sync.WaitGroup
	wg.Add(2)
	writeErr := make(chan error, 1)
	var sawOn, sawOff bool

	go func() { // writer: flip the coupled state on and off
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := flip(true); err != nil {
				writeErr <- err
				return
			}
			if err := flip(false); err != nil {
				writeErr <- err
				return
			}
		}
	}()

	go func() { // reader: the invariant must hold in every snapshot
		defer wg.Done()
		for i := 0; i < iterations*4; i++ {
			view, err := reader.GetIssueView(iss.ID)
			if err != nil {
				t.Errorf("GetIssueView: %v", err)
				return
			}
			on := view.Owner == "on"
			has := containsLabel(view.Labels, "toggle")
			if on != has {
				t.Errorf("torn snapshot: owner=%q, labels=%v (owner and label must agree)", view.Owner, view.Labels)
				return
			}
			if on {
				sawOn = true
			} else {
				sawOff = true
			}
		}
	}()

	wg.Wait()
	select {
	case err := <-writeErr:
		t.Fatalf("writer: %v", err)
	default:
	}
	// Prove the goroutines actually overlapped and both committed states were
	// observed, so the invariant check was meaningfully exercised.
	if !sawOn || !sawOff {
		t.Fatalf("reader did not observe both states (on=%v, off=%v); no real overlap", sawOn, sawOff)
	}
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}
