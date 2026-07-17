package service_test

import (
	"errors"
	"testing"

	"github.com/brent-hoover/sutra/internal/domain"
)

// Adding then removing a label updates the derived labels list, advances the
// issue's updated_at, and appends an "updated" ledger entry each time.
func TestLabelAddRemoveAndLog(t *testing.T) {
	svc := newService(t, t.TempDir())
	iss := mkIssue(t, svc, "Labelled")
	t0 := issueUpdatedAt(t, svc, iss.ID)

	got, err := svc.AddLabel(iss.ID, "backend")
	if err != nil {
		t.Fatalf("add label: %v", err)
	}
	if len(got.Labels) != 1 || got.Labels[0] != "backend" {
		t.Fatalf("labels = %v, want [backend]", got.Labels)
	}
	t1 := issueUpdatedAt(t, svc, iss.ID)
	if !t1.After(t0) {
		t.Errorf("add label did not advance updated_at")
	}
	if n := countLedger(t, svc, iss.ID, domain.LedgerUpdated); n != 1 {
		t.Errorf("updated ledger entries after add = %d, want 1", n)
	}

	// Adding the same label again is a no-op: no new ledger entry, no bump.
	if _, err := svc.AddLabel(iss.ID, "backend"); err != nil {
		t.Fatalf("re-add label: %v", err)
	}
	if n := countLedger(t, svc, iss.ID, domain.LedgerUpdated); n != 1 {
		t.Errorf("updated ledger entries after re-add = %d, want 1 (idempotent)", n)
	}

	got, err = svc.RemoveLabel(iss.ID, "backend")
	if err != nil {
		t.Fatalf("remove label: %v", err)
	}
	if len(got.Labels) != 0 {
		t.Fatalf("labels after remove = %v, want empty", got.Labels)
	}
	t2 := issueUpdatedAt(t, svc, iss.ID)
	if !t2.After(t1) {
		t.Errorf("remove label did not advance updated_at")
	}
	if n := countLedger(t, svc, iss.ID, domain.LedgerUpdated); n != 2 {
		t.Errorf("updated ledger entries after remove = %d, want 2", n)
	}
}

func TestLabelValidationAndNotFound(t *testing.T) {
	svc := newService(t, t.TempDir())
	iss := mkIssue(t, svc, "Labelled")

	if _, err := svc.AddLabel(iss.ID, ""); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("empty label: err = %v, want ErrInvalidIssue", err)
	}
	if _, err := svc.AddLabel("nope", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing issue: err = %v, want ErrNotFound", err)
	}
}

// The label filter narrows the list to issues carrying the label (AND with the
// other filters), excluding soft-deleted issues.
func TestListFilterByLabel(t *testing.T) {
	svc := newService(t, t.TempDir())
	a := mkIssue(t, svc, "A")
	b := mkIssue(t, svc, "B")
	c := mkIssue(t, svc, "C")
	for _, id := range []string{a.ID, b.ID} {
		if _, err := svc.AddLabel(id, "backend"); err != nil {
			t.Fatalf("label %s: %v", id, err)
		}
	}

	got, err := svc.ListIssues(domain.IssueFilter{Label: "backend"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("label filter returned %d issues, want 2", len(got))
	}
	ids := map[string]bool{}
	for _, i := range got {
		ids[i.ID] = true
	}
	if !ids[a.ID] || !ids[b.ID] || ids[c.ID] {
		t.Errorf("label filter returned wrong set: %v", ids)
	}

	// Soft-deleting a labelled issue drops it from the filtered list.
	if _, err := svc.SoftDeleteIssue(a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, err = svc.ListIssues(domain.IssueFilter{Label: "backend"})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(got) != 1 || got[0].ID != b.ID {
		t.Errorf("after delete, label filter = %v, want [%s]", got, b.ID)
	}
}
