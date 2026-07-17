package store

// White-box test: it uses the unexported afterIssueReadHook seam and reaches
// into a second handle's db to prove — deterministically — that GetIssueView
// reads the issue row and its derived collections from one consistent snapshot.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// TestGetIssueViewSnapshotIsolatesCommit proves the GetIssueView transaction
// isolates the view from a write that commits mid-read.
//
// The invariant owner=="on" IFF the "toggle" label is present holds in every
// committed DB state (the writer flips both atomically). Using the
// afterIssueReadHook seam, a writer on a second handle commits the on-state
// after GetIssueView has read the issue row but before it reads the labels.
// A transactional view reads both from the pre-commit snapshot (owner=off, no
// label) — the invariant holds. A non-transactional view would read the row
// pre-commit and the labels post-commit — owner=off but label present — and
// violate it. The write is then confirmed to have landed, proving the commit
// genuinely occurred between the two reads.
func TestGetIssueViewSnapshotIsolatesCommit(t *testing.T) {
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

	// Atomically move to the on-state: owner="on" and the "toggle" label present.
	flipOn := func() error {
		tx, err := writer.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE issues SET owner = 'on' WHERE id = ?`, iss.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO issue_label (issue_id, label) VALUES (?, 'toggle')`, iss.ID); err != nil {
			return err
		}
		return tx.Commit()
	}

	// Commit the on-state exactly once, from the second handle, between the view's
	// issue-row read and its label read.
	fired := false
	var hookErr error
	afterIssueReadHook = func() {
		if fired {
			return
		}
		fired = true
		hookErr = flipOn()
	}
	t.Cleanup(func() { afterIssueReadHook = nil })

	view, err := reader.GetIssueView(iss.ID)
	afterIssueReadHook = nil
	if err != nil {
		t.Fatalf("GetIssueView: %v", err)
	}
	if hookErr != nil {
		t.Fatalf("mid-read commit: %v", hookErr)
	}
	if !fired {
		t.Fatal("hook did not fire; the mid-read commit was not exercised")
	}

	// The view must be internally consistent (a single snapshot).
	on := view.Owner == "on"
	has := containsLabel(view.Labels, "toggle")
	if on != has {
		t.Fatalf("torn snapshot: owner=%q, labels=%v (owner and label must agree)", view.Owner, view.Labels)
	}
	// It must reflect the pre-commit snapshot, not the write that landed mid-read.
	if on {
		t.Fatalf("view observed the mid-read commit; snapshot not isolated (owner=%q, labels=%v)", view.Owner, view.Labels)
	}

	// Confirm the write really did commit during the view read.
	after, err := reader.GetIssueView(iss.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Owner != "on" || !containsLabel(after.Labels, "toggle") {
		t.Fatalf("mid-read commit did not persist: owner=%q, labels=%v", after.Owner, after.Labels)
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
