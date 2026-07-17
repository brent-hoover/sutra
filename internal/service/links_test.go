package service_test

import (
	"errors"
	"testing"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

// mkIssue creates an issue and fails the test on error.
func mkIssue(t *testing.T, svc *service.Service, subject string) domain.Issue {
	t.Helper()
	iss, err := svc.CreateIssue(subject, "body")
	if err != nil {
		t.Fatalf("create %s: %v", subject, err)
	}
	return iss
}

func countLedger(t *testing.T, svc *service.Service, issueID string, kind domain.LedgerKind) int {
	t.Helper()
	entries, err := svc.IssueHistory(issueID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// SetParent establishes the parent/child relation, advances updated_at, and
// appends a "linked" ledger entry recording the old and new parent.
func TestSetParentSucceedsAndLogs(t *testing.T) {
	svc := newService(t, t.TempDir())
	parent := mkIssue(t, svc, "Parent")
	child := mkIssue(t, svc, "Child")
	t0 := issueUpdatedAt(t, svc, child.ID)

	got, err := svc.SetParent(child.ID, parent.ID)
	if err != nil {
		t.Fatalf("set parent: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Fatalf("parent_id = %v, want %q", got.ParentID, parent.ID)
	}
	if !issueUpdatedAt(t, svc, child.ID).After(t0) {
		t.Errorf("set parent did not advance child updated_at")
	}
	if n := countLedger(t, svc, child.ID, domain.LedgerLinked); n != 1 {
		t.Errorf("linked ledger entries = %d, want 1", n)
	}
	entries, _ := svc.IssueHistory(child.ID)
	for _, e := range entries {
		if e.Kind == domain.LedgerLinked && e.Field == "parent_id" && e.NewValue != parent.ID {
			t.Errorf("linked entry new_value = %q, want %q", e.NewValue, parent.ID)
		}
	}
}

func TestSetParentRejectsSelfAndCycles(t *testing.T) {
	svc := newService(t, t.TempDir())
	a := mkIssue(t, svc, "A")
	b := mkIssue(t, svc, "B")
	c := mkIssue(t, svc, "C")

	// Self-parent.
	if _, err := svc.SetParent(a.ID, a.ID); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("self-parent: err = %v, want ErrInvalidIssue", err)
	}

	// A→B→A: B's parent is A, then A's parent = B closes a 2-cycle.
	if _, err := svc.SetParent(b.ID, a.ID); err != nil {
		t.Fatalf("set B parent A: %v", err)
	}
	if _, err := svc.SetParent(a.ID, b.ID); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("2-cycle: err = %v, want ErrInvalidIssue", err)
	}

	// A→B→C chain (A parent of B, B parent of C); A's parent = C closes a cycle.
	if _, err := svc.SetParent(c.ID, b.ID); err != nil {
		t.Fatalf("set C parent B: %v", err)
	}
	if _, err := svc.SetParent(a.ID, c.ID); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("3-cycle: err = %v, want ErrInvalidIssue", err)
	}
}

// Re-setting the same parent is an idempotent no-op (no second ledger entry, no
// updated_at bump), and an empty parent id is rejected.
func TestSetParentIdempotentAndEmptyRejected(t *testing.T) {
	svc := newService(t, t.TempDir())
	parent := mkIssue(t, svc, "Parent")
	child := mkIssue(t, svc, "Child")

	if _, err := svc.SetParent(child.ID, parent.ID); err != nil {
		t.Fatalf("set parent: %v", err)
	}
	t1 := issueUpdatedAt(t, svc, child.ID)

	// Repeat the same parent: nothing should change.
	if _, err := svc.SetParent(child.ID, parent.ID); err != nil {
		t.Fatalf("re-set parent: %v", err)
	}
	if got := issueUpdatedAt(t, svc, child.ID); !got.Equal(t1) {
		t.Errorf("re-set parent advanced updated_at: %s != %s", got, t1)
	}
	if n := countLedger(t, svc, child.ID, domain.LedgerLinked); n != 1 {
		t.Errorf("linked ledger entries after re-set = %d, want 1 (idempotent)", n)
	}

	// Empty parent id is invalid.
	if _, err := svc.SetParent(child.ID, ""); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("empty parent: err = %v, want ErrInvalidIssue", err)
	}
}

func TestSetParentMissingIssue(t *testing.T) {
	svc := newService(t, t.TempDir())
	a := mkIssue(t, svc, "A")
	if _, err := svc.SetParent(a.ID, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing parent: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.SetParent("nope", a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing child: err = %v, want ErrNotFound", err)
	}
}

// Blocking is directional: the blocked issue shows the blocker in blocked_by and
// the blocker shows the blocked issue in is_blocking (derived from issue_block).
func TestBlockDerivedEdges(t *testing.T) {
	svc := newService(t, t.TempDir())
	a := mkIssue(t, svc, "A")
	b := mkIssue(t, svc, "B")
	t0a := issueUpdatedAt(t, svc, a.ID)
	t0b := issueUpdatedAt(t, svc, b.ID)

	if _, err := svc.BlockIssue(a.ID, b.ID); err != nil { // A blocked by B
		t.Fatalf("block: %v", err)
	}

	va, err := svc.GetIssueView(a.ID)
	if err != nil {
		t.Fatalf("view A: %v", err)
	}
	if len(va.BlockedBy) != 1 || va.BlockedBy[0] != b.ID {
		t.Errorf("A.blocked_by = %v, want [%s]", va.BlockedBy, b.ID)
	}
	if len(va.IsBlocking) != 0 {
		t.Errorf("A.is_blocking = %v, want empty", va.IsBlocking)
	}
	vb, err := svc.GetIssueView(b.ID)
	if err != nil {
		t.Fatalf("view B: %v", err)
	}
	if len(vb.IsBlocking) != 1 || vb.IsBlocking[0] != a.ID {
		t.Errorf("B.is_blocking = %v, want [%s]", vb.IsBlocking, a.ID)
	}

	// Both endpoints changed, so both updated_at advance, and the acting (blocked)
	// issue records a linked ledger entry.
	if !issueUpdatedAt(t, svc, a.ID).After(t0a) {
		t.Errorf("block did not advance blocked issue updated_at")
	}
	if !issueUpdatedAt(t, svc, b.ID).After(t0b) {
		t.Errorf("block did not advance blocker issue updated_at")
	}
	if n := countLedger(t, svc, a.ID, domain.LedgerLinked); n != 1 {
		t.Errorf("blocked issue linked ledger entries = %d, want 1", n)
	}

	if _, err := svc.BlockIssue(a.ID, a.ID); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("self-block: err = %v, want ErrInvalidIssue", err)
	}
}

// Relating is symmetric: each issue appears in the other's related list, both
// updated_at advance, and the acting issue records a linked ledger entry.
func TestRelateSymmetric(t *testing.T) {
	svc := newService(t, t.TempDir())
	a := mkIssue(t, svc, "A")
	b := mkIssue(t, svc, "B")
	t0a := issueUpdatedAt(t, svc, a.ID)
	t0b := issueUpdatedAt(t, svc, b.ID)

	if _, err := svc.RelateIssues(a.ID, b.ID); err != nil {
		t.Fatalf("relate: %v", err)
	}
	va, _ := svc.GetIssueView(a.ID)
	vb, _ := svc.GetIssueView(b.ID)
	if len(va.Related) != 1 || va.Related[0] != b.ID {
		t.Errorf("A.related = %v, want [%s]", va.Related, b.ID)
	}
	if len(vb.Related) != 1 || vb.Related[0] != a.ID {
		t.Errorf("B.related = %v, want [%s]", vb.Related, a.ID)
	}
	if !issueUpdatedAt(t, svc, a.ID).After(t0a) || !issueUpdatedAt(t, svc, b.ID).After(t0b) {
		t.Errorf("relate did not advance both issues' updated_at")
	}
	if n := countLedger(t, svc, a.ID, domain.LedgerLinked); n != 1 {
		t.Errorf("acting issue linked ledger entries = %d, want 1", n)
	}

	// Relating again is a no-op (no second edge, no second ledger entry).
	if _, err := svc.RelateIssues(b.ID, a.ID); err != nil {
		t.Fatalf("re-relate: %v", err)
	}
	va2, _ := svc.GetIssueView(a.ID)
	if len(va2.Related) != 1 {
		t.Errorf("A.related after re-relate = %v, want single edge", va2.Related)
	}
	if _, err := svc.RelateIssues(a.ID, a.ID); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("self-relate: err = %v, want ErrInvalidIssue", err)
	}
}
