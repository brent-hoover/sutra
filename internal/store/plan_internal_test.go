package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// buildTestPlan assembles a plan issue + n tracer children (ParentID set to the
// plan, one created ledger entry each) and calls BuildPlan. parentID/projectID
// are optional references on the plan itself.
func buildTestPlan(t *testing.T, s *Store, n int, parentID, projectID *string) (domain.Issue, []domain.Issue) {
	t.Helper()
	now := time.Now().UTC()
	plan := domain.Issue{
		ID: domain.NewID(), Subject: "the plan", Body: "overall prose",
		Type: domain.TypePlan, Status: domain.StatusOpen, Priority: domain.P2,
		Approval: domain.ApprovalPending, ParentID: parentID, ProjectID: projectID,
		CreatedAt: now, UpdatedAt: now,
	}
	ledger := []domain.LedgerEntry{{ID: domain.NewID(), IssueID: plan.ID, At: now, Kind: domain.LedgerCreated}}
	children := make([]domain.Issue, n)
	for i := range children {
		children[i] = domain.Issue{
			ID: fmt.Sprintf("tracer-%02d-%s", n-i, domain.NewID()), Subject: "tracer", Body: "b",
			Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
			ParentID: &plan.ID, CreatedAt: now, UpdatedAt: now,
		}
		ledger = append(ledger, domain.LedgerEntry{ID: domain.NewID(), IssueID: children[i].ID, At: now, Kind: domain.LedgerCreated})
	}
	if err := s.BuildPlan(plan, children, ledger); err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	return plan, children
}

func TestBuildPlanCreatesChainedTreeInOrder(t *testing.T) {
	s := openStore(t)
	plan, children := buildTestPlan(t, s, 3, nil, nil)

	got, err := s.GetIssue(plan.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if got.Type != domain.TypePlan || got.Approval != domain.ApprovalPending {
		t.Fatalf("plan type/approval = %q/%q", got.Type, got.Approval)
	}
	// Each child parents the plan; BuildPlan stamps created_at in run order.
	var last time.Time
	for i, c := range children {
		cv, err := s.GetIssue(c.ID)
		if err != nil {
			t.Fatalf("get child %d: %v", i, err)
		}
		if cv.ParentID == nil || *cv.ParentID != plan.ID {
			t.Errorf("child %d parent = %v, want %s", i, cv.ParentID, plan.ID)
		}
		if i > 0 && !cv.CreatedAt.After(last) {
			t.Errorf("child %d created_at %v not after previous %v", i, cv.CreatedAt, last)
		}
		last = cv.CreatedAt
	}
	// Chain: child1 blocked by child0, child2 blocked by child1.
	v1, _ := s.GetIssueView(children[1].ID)
	if len(v1.BlockedBy) != 1 || v1.BlockedBy[0] != children[0].ID {
		t.Errorf("child1 blocked_by = %v, want [%s]", v1.BlockedBy, children[0].ID)
	}
	v2, _ := s.GetIssueView(children[2].ID)
	if len(v2.BlockedBy) != 1 || v2.BlockedBy[0] != children[1].ID {
		t.Errorf("child2 blocked_by = %v, want [%s]", v2.BlockedBy, children[1].ID)
	}
}

func TestBuildPlanInheritsParentProject(t *testing.T) {
	s := openStore(t)
	p := newProject(t, s, "beta", "/repos/beta")
	now := time.Now().UTC()
	feat := domain.Issue{
		ID: domain.NewID(), Subject: "feature", Body: "b",
		Type: domain.TypeFeature, Status: domain.StatusOpen, Priority: domain.P2,
		ProjectID: &p.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(feat, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: feat.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create feature: %v", err)
	}
	plan, children := buildTestPlan(t, s, 2, &feat.ID, nil)

	gp, _ := s.GetIssue(plan.ID)
	if gp.ProjectID == nil || *gp.ProjectID != p.ID {
		t.Errorf("plan project = %v, want %s", gp.ProjectID, p.ID)
	}
	for _, c := range children {
		gc, _ := s.GetIssue(c.ID)
		if gc.ProjectID == nil || *gc.ProjectID != p.ID {
			t.Errorf("child project = %v, want %s", gc.ProjectID, p.ID)
		}
	}
}

func TestBuildPlanRejectsMissingProject(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	plan := domain.Issue{
		ID: domain.NewID(), Subject: "p", Body: "b", Type: domain.TypePlan,
		Status: domain.StatusOpen, Priority: domain.P2, Approval: domain.ApprovalPending,
		ProjectID: strptr("missing"), CreatedAt: now, UpdatedAt: now,
	}
	child := domain.Issue{
		ID: domain.NewID(), Subject: "c", Body: "b", Type: domain.TypeTask,
		Status: domain.StatusOpen, Priority: domain.P2, ParentID: &plan.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	err := s.BuildPlan(plan, []domain.Issue{child},
		[]domain.LedgerEntry{{ID: domain.NewID(), IssueID: plan.ID, At: now, Kind: domain.LedgerCreated}})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("BuildPlan err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetIssue(plan.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("plan was persisted despite rollback")
	}
}

func TestBuildPlanRejectsParentProjectConflict(t *testing.T) {
	s := openStore(t)
	parentProject := newProject(t, s, "parent", "/repos/parent")
	otherProject := newProject(t, s, "other", "/repos/other")
	now := time.Now().UTC()
	parent := domain.Issue{
		ID: domain.NewID(), Subject: "feature", Body: "b",
		Type: domain.TypeFeature, Status: domain.StatusOpen, Priority: domain.P2,
		ProjectID: &parentProject.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(parent, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: parent.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	plan := domain.Issue{
		ID: domain.NewID(), Subject: "p", Body: "b", Type: domain.TypePlan,
		Status: domain.StatusOpen, Priority: domain.P2, Approval: domain.ApprovalPending,
		ParentID: &parent.ID, ProjectID: &otherProject.ID, CreatedAt: now, UpdatedAt: now,
	}
	child := domain.Issue{
		ID: domain.NewID(), Subject: "c", Body: "b", Type: domain.TypeTask,
		Status: domain.StatusOpen, Priority: domain.P2, ParentID: &plan.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	err := s.BuildPlan(plan, []domain.Issue{child},
		[]domain.LedgerEntry{{ID: domain.NewID(), IssueID: plan.ID, At: now, Kind: domain.LedgerCreated}})
	if !errors.Is(err, domain.ErrInvalidIssue) {
		t.Fatalf("BuildPlan err = %v, want ErrInvalidIssue", err)
	}
	if _, err := s.GetIssue(plan.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("plan was persisted despite rollback")
	}
}

func TestBuildPlanRollsBackAfterLedgerFailure(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	plan := domain.Issue{
		ID: domain.NewID(), Subject: "p", Body: "b", Type: domain.TypePlan,
		Status: domain.StatusOpen, Priority: domain.P2, Approval: domain.ApprovalPending,
		CreatedAt: now, UpdatedAt: now,
	}
	children := []domain.Issue{
		{
			ID: domain.NewID(), Subject: "c1", Body: "b", Type: domain.TypeTask,
			Status: domain.StatusOpen, Priority: domain.P2, ParentID: &plan.ID,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: domain.NewID(), Subject: "c2", Body: "b", Type: domain.TypeTask,
			Status: domain.StatusOpen, Priority: domain.P2, ParentID: &plan.ID,
			CreatedAt: now, UpdatedAt: now,
		},
	}
	ledgerID := domain.NewID()
	err := s.BuildPlan(plan, children, []domain.LedgerEntry{
		{ID: ledgerID, IssueID: plan.ID, At: now, Kind: domain.LedgerCreated},
		{ID: ledgerID, IssueID: children[0].ID, At: now, Kind: domain.LedgerCreated},
	})
	if err == nil {
		t.Fatalf("BuildPlan err = nil, want duplicate ledger failure")
	}
	if _, err := s.GetIssue(plan.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("plan was persisted despite rollback")
	}
	for _, child := range children {
		if _, err := s.GetIssue(child.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("child %s was persisted despite rollback", child.ID)
		}
	}
	var edges int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM issue_block WHERE blocker_id = ? AND blocked_id = ?`,
		children[0].ID, children[1].ID,
	).Scan(&edges); err != nil {
		t.Fatalf("count block edges: %v", err)
	}
	if edges != 0 {
		t.Errorf("block edge count = %d, want 0", edges)
	}
	var ledgerRows int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ledger WHERE id = ? OR issue_id IN (?, ?, ?)`,
		ledgerID, plan.ID, children[0].ID, children[1].ID,
	).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 0 {
		t.Errorf("ledger rows = %d, want 0", ledgerRows)
	}
}

func TestApprovePlanApprovesAndLedgers(t *testing.T) {
	s := openStore(t)
	plan, _ := buildTestPlan(t, s, 1, nil, nil)

	entry := domain.LedgerEntry{ID: domain.NewID(), IssueID: plan.ID, Kind: domain.LedgerUpdated, Field: "approval", NewValue: string(domain.ApprovalApproved)}
	got, err := s.ApprovePlan(plan.ID, entry)
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if got.Approval != domain.ApprovalApproved {
		t.Errorf("approval = %q, want approved", got.Approval)
	}
	hist, _ := s.LedgerFor(plan.ID)
	var found bool
	for _, e := range hist {
		if e.Kind == domain.LedgerUpdated && e.Field == "approval" && e.NewValue == string(domain.ApprovalApproved) {
			found = true
		}
	}
	if !found {
		t.Errorf("no approval ledger entry in %v", hist)
	}
}

func TestApprovePlanIdempotent(t *testing.T) {
	s := openStore(t)
	plan, _ := buildTestPlan(t, s, 1, nil, nil)
	entry := func() domain.LedgerEntry {
		return domain.LedgerEntry{ID: domain.NewID(), IssueID: plan.ID, Kind: domain.LedgerUpdated, Field: "approval", NewValue: string(domain.ApprovalApproved)}
	}
	if _, err := s.ApprovePlan(plan.ID, entry()); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	before, _ := s.LedgerFor(plan.ID)
	if _, err := s.ApprovePlan(plan.ID, entry()); err != nil {
		t.Fatalf("second approve: %v", err)
	}
	after, _ := s.LedgerFor(plan.ID)
	if len(after) != len(before) {
		t.Errorf("ledger grew on idempotent approve: %d -> %d", len(before), len(after))
	}
}

func TestApprovePlanRejectsInvalidTargets(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	entry := func(id string) domain.LedgerEntry {
		return domain.LedgerEntry{ID: domain.NewID(), IssueID: id, Kind: domain.LedgerUpdated, Field: "approval"}
	}
	task := domain.Issue{
		ID: domain.NewID(), Subject: "t", Body: "b", Type: domain.TypeTask,
		Status: domain.StatusOpen, Priority: domain.P2, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(task, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: task.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := s.ApprovePlan(task.ID, entry(task.ID)); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("approve task err = %v, want ErrInvalidIssue", err)
	}
	for _, approval := range []domain.Approval{"", "reviewing"} {
		plan := domain.Issue{
			ID: domain.NewID(), Subject: "p", Body: "b", Type: domain.TypePlan,
			Status: domain.StatusOpen, Priority: domain.P2, Approval: approval,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.CreateIssue(plan, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: plan.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
			t.Fatalf("create plan with approval %q: %v", approval, err)
		}
		if _, err := s.ApprovePlan(plan.ID, entry(plan.ID)); !errors.Is(err, domain.ErrInvalidIssue) {
			t.Errorf("approve plan with approval %q err = %v, want ErrInvalidIssue", approval, err)
		}
	}
	if _, err := s.ApprovePlan("missing", entry("missing")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("approve missing err = %v, want ErrNotFound", err)
	}
}

func TestListByParentReturnsChildrenInOrder(t *testing.T) {
	s := openStore(t)
	plan, children := buildTestPlan(t, s, 3, nil, nil)

	// A second, unrelated plan whose children must NOT leak into the first's list.
	_, _ = buildTestPlan(t, s, 2, nil, nil)

	got, err := s.ListIssues(domain.IssueFilter{ParentID: plan.ID})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d children, want 3", len(got))
	}
	for i, c := range children {
		if got[i].ID != c.ID {
			t.Errorf("child %d = %s, want %s (run order)", i, got[i].ID, c.ID)
		}
	}
}

func strptr(s string) *string { return &s }

// A plain issue created without an approval reads back with approval "" — the
// new column round-trips and defaults empty for non-plan issues.
func TestApprovalColumnRoundTripsEmpty(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "plain", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(iss, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	got, err := s.GetIssue(iss.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if got.Approval != "" {
		t.Errorf("approval = %q, want empty", got.Approval)
	}
}

func TestApprovalMigrationIsIdempotentOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := first.db.Exec(`SELECT approval FROM issues LIMIT 0`); err != nil {
		t.Fatalf("approval column missing after first open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	t.Cleanup(func() { second.Close() })
	if _, err := second.db.Exec(`SELECT approval FROM issues LIMIT 0`); err != nil {
		t.Fatalf("approval column missing after reopen: %v", err)
	}
}
