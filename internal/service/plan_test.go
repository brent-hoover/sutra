package service_test

import (
	"errors"
	"testing"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

func TestBuildPlanCreatesTreeWithDefaults(t *testing.T) {
	svc := newService(t, t.TempDir())
	steps := []service.PlanStep{
		{Subject: "first"}, // defaults: task/p2, body←subject
		{Subject: "second", Body: "do the second thing"},                //
		{Subject: "third", Type: domain.TypeChore, Priority: domain.P1}, // explicit type/priority
	}
	plan, children, err := svc.BuildPlan("Search feature", "overall prose", steps, nil, nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Type != domain.TypePlan || plan.Approval != domain.ApprovalPending {
		t.Errorf("plan type/approval = %q/%q, want plan/pending", plan.Type, plan.Approval)
	}
	if plan.Subject != "Search feature" || plan.Body != "overall prose" {
		t.Errorf("plan subject/body = %q/%q", plan.Subject, plan.Body)
	}
	if len(children) != 3 {
		t.Fatalf("children = %d, want 3", len(children))
	}
	if children[0].Type != domain.TypeTask || children[0].Priority != domain.P2 {
		t.Errorf("child0 defaults = %q/%q, want task/p2", children[0].Type, children[0].Priority)
	}
	if children[0].Body != "first" {
		t.Errorf("child0 body = %q, want subject fallback %q", children[0].Body, "first")
	}
	if children[2].Type != domain.TypeChore || children[2].Priority != domain.P1 {
		t.Errorf("child2 = %q/%q, want chore/p1", children[2].Type, children[2].Priority)
	}
	for i, c := range children {
		if c.ParentID == nil || *c.ParentID != plan.ID {
			t.Errorf("child %d parent = %v, want %s", i, c.ParentID, plan.ID)
		}
	}
	// Sequential chain: child1 blocked by child0.
	v, err := svc.GetIssueView(children[1].ID)
	if err != nil {
		t.Fatalf("view child1: %v", err)
	}
	if len(v.BlockedBy) != 1 || v.BlockedBy[0] != children[0].ID {
		t.Errorf("child1 blocked_by = %v, want [%s]", v.BlockedBy, children[0].ID)
	}
}

func TestBuildPlanValidation(t *testing.T) {
	svc := newService(t, t.TempDir())
	ok := []service.PlanStep{{Subject: "a"}}
	cases := []struct {
		name         string
		title, prose string
		steps        []service.PlanStep
	}{
		{"empty title", "", "prose", ok},
		{"empty prose", "t", "", ok},
		{"no steps", "t", "prose", nil},
		{"empty subject", "t", "prose", []service.PlanStep{{Subject: ""}}},
		{"bad type", "t", "prose", []service.PlanStep{{Subject: "a", Type: domain.IssueType("epic")}}},
		{"plan type", "t", "prose", []service.PlanStep{{Subject: "a", Type: domain.TypePlan}}},
		{"bad priority", "t", "prose", []service.PlanStep{{Subject: "a", Priority: domain.Priority("p9")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.BuildPlan(tc.title, tc.prose, tc.steps, nil, nil)
			if !errors.Is(err, domain.ErrInvalidIssue) {
				t.Fatalf("err = %v, want ErrInvalidIssue", err)
			}
		})
	}
}

func TestBuildPlanLinksParent(t *testing.T) {
	svc := newService(t, t.TempDir())
	feat, err := svc.CreateIssue("feature", "b")
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}
	plan, _, err := svc.BuildPlan("plan", "prose", []service.PlanStep{{Subject: "a"}}, &feat.ID, nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.ParentID == nil || *plan.ParentID != feat.ID {
		t.Errorf("plan parent = %v, want %s", plan.ParentID, feat.ID)
	}
}

func TestUpdateIssueRejectsPlanTypeTransitions(t *testing.T) {
	svc := newService(t, t.TempDir())
	task, err := svc.CreateIssue("task", "b")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	toPlan := domain.TypePlan
	if _, err := svc.UpdateIssue(task.ID, service.IssueUpdate{Type: &toPlan}); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Fatalf("task -> plan err = %v, want ErrInvalidIssue", err)
	}

	plan, _, err := svc.BuildPlan("plan", "prose", []service.PlanStep{{Subject: "a"}}, nil, nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if _, err := svc.ApprovePlan(plan.ID); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	toTask := domain.TypeTask
	if _, err := svc.UpdateIssue(plan.ID, service.IssueUpdate{Type: &toTask}); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Fatalf("plan -> task err = %v, want ErrInvalidIssue", err)
	}
	got, err := svc.GetIssue(plan.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if got.Type != domain.TypePlan || got.Approval != domain.ApprovalApproved {
		t.Errorf("plan type/approval = %q/%q, want plan/approved", got.Type, got.Approval)
	}
}

func TestApprovePlanServiceLayer(t *testing.T) {
	svc := newService(t, t.TempDir())
	plan, _, err := svc.BuildPlan("plan", "prose", []service.PlanStep{{Subject: "a"}}, nil, nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	got, err := svc.ApprovePlan(plan.ID)
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if got.Approval != domain.ApprovalApproved {
		t.Errorf("approval = %q, want approved", got.Approval)
	}

	task, err := svc.CreateIssue("task", "b")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.ApprovePlan(task.ID); !errors.Is(err, domain.ErrInvalidIssue) {
		t.Errorf("approve task err = %v, want ErrInvalidIssue", err)
	}
	if _, err := svc.ApprovePlan("missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("approve missing err = %v, want ErrNotFound", err)
	}
}
