package store

// White-box tests: they use the unexported afterIssueReadHook seam to prove
// deterministically that GetIssue and ListIssues read an issue row and its
// derived labels from one consistent snapshot.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// seedToggleIssue creates an issue in the "off" state (owner="off", no "toggle"
// label) and returns its id plus a flipOn func (run on a second handle) that
// atomically moves it to the "on" state (owner="on", "toggle" label present).
// The invariant owner=="on" IFF the "toggle" label is present holds in every
// committed state.
func seedToggleIssue(t *testing.T) (string, *Store, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snap.db")
	reader, err := Open(path)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { reader.Close() })

	now := time.Now().UTC()
	id := domain.NewID()
	iss := domain.Issue{
		ID: id, Subject: "s", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		Owner: "off", CreatedAt: now, UpdatedAt: now,
	}
	if err := reader.CreateIssue(iss, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: id, At: now, Kind: domain.LedgerCreated,
	}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	writer, err := Open(path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { writer.Close() })

	flipOn := func() {
		tx, err := writer.db.Begin()
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE issues SET owner = 'on' WHERE id = ?`, id); err != nil {
			t.Fatalf("set owner: %v", err)
		}
		if _, err := tx.Exec(`INSERT INTO issue_label (issue_id, label) VALUES (?, 'toggle')`, id); err != nil {
			t.Fatalf("add label: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	return id, reader, flipOn
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func TestGetIssueSnapshotIsolatesCommit(t *testing.T) {
	id, reader, flipOn := seedToggleIssue(t)

	// Commit the on-state after the row is read but before labels are read.
	afterIssueReadHook = func() {
		flipOn()
		afterIssueReadHook = nil // fire once
	}
	t.Cleanup(func() { afterIssueReadHook = nil })

	got, err := reader.GetIssue(id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	// A non-transactional read would see owner="off" (pre-commit) but the
	// "toggle" label (post-commit), breaking the invariant.
	if (got.Owner == "on") != hasLabel(got.Labels, "toggle") {
		t.Errorf("inconsistent snapshot: owner=%q labels=%v", got.Owner, got.Labels)
	}
}

func TestListIssuesSnapshotIsolatesCommit(t *testing.T) {
	id, reader, flipOn := seedToggleIssue(t)

	afterIssueReadHook = func() {
		flipOn()
		afterIssueReadHook = nil
	}
	t.Cleanup(func() { afterIssueReadHook = nil })

	got, err := reader.ListIssues(domain.IssueFilter{})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	var found bool
	for _, is := range got {
		if is.ID != id {
			continue
		}
		found = true
		if (is.Owner == "on") != hasLabel(is.Labels, "toggle") {
			t.Errorf("inconsistent snapshot: owner=%q labels=%v", is.Owner, is.Labels)
		}
	}
	if !found {
		t.Fatalf("issue %s not in list", id)
	}
}
