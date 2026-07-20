package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// PlanStep is one tracer item supplied by the caller when building a plan. Type
// and Priority are optional; empty means the default (task / p2). An empty Body
// defaults to the Subject so the non-empty-body invariant holds.
type PlanStep struct {
	Subject  string
	Body     string
	Type     domain.IssueType
	Priority domain.Priority
}

// BuildPlan creates a plan issue (type plan, approval pending, prose as its body)
// and one tracer child per step, chained in order (child i blocks child i+1),
// atomically. parentID (the feature issue being planned) and projectID are
// optional references; when parentID is set and projectID is nil the plan and its
// children inherit the parent's project. Returns ErrInvalidIssue for empty
// title/prose/steps, an empty step subject, or an invalid enum; ErrNotFound if a
// supplied parent or project does not exist. The returned plan and children are
// re-read so their inherited project is reflected.
func (s *Service) BuildPlan(title, prose string, steps []PlanStep, parentID, projectID *string) (domain.Issue, []domain.Issue, error) {
	if title == "" {
		return domain.Issue{}, nil, errors.Join(domain.ErrInvalidIssue, errors.New("plan title is required"))
	}
	if prose == "" {
		return domain.Issue{}, nil, errors.Join(domain.ErrInvalidIssue, errors.New("plan prose is required"))
	}
	if len(steps) == 0 {
		return domain.Issue{}, nil, errors.Join(domain.ErrInvalidIssue, errors.New("at least one tracer item is required"))
	}
	for i, step := range steps {
		if step.Subject == "" {
			return domain.Issue{}, nil, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("tracer item %d has an empty subject", i+1))
		}
		if step.Type != "" && !step.Type.Valid() {
			return domain.Issue{}, nil, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("tracer item %d has invalid type %q", i+1, step.Type))
		}
		if step.Priority != "" && !step.Priority.Valid() {
			return domain.Issue{}, nil, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("tracer item %d has invalid priority %q", i+1, step.Priority))
		}
	}

	now := time.Now().UTC()
	plan := domain.Issue{
		ID: domain.NewID(), Subject: title, Body: prose,
		Type: domain.TypePlan, Status: domain.StatusOpen, Priority: domain.P2,
		Approval: domain.ApprovalPending, ParentID: parentID, ProjectID: projectID,
		CreatedAt: now, UpdatedAt: now,
	}
	ledger := []domain.LedgerEntry{{ID: domain.NewID(), IssueID: plan.ID, At: now, Kind: domain.LedgerCreated}}

	children := make([]domain.Issue, len(steps))
	for i, step := range steps {
		// Strictly-increasing created_at gives the children a deterministic run
		// order under ListIssues' `ORDER BY created_at, id`.
		at := now.Add(time.Duration(i + 1))
		typ := step.Type
		if typ == "" {
			typ = domain.TypeTask
		}
		pri := step.Priority
		if pri == "" {
			pri = domain.P2
		}
		body := step.Body
		if body == "" {
			body = step.Subject
		}
		children[i] = domain.Issue{
			ID: domain.NewID(), Subject: step.Subject, Body: body,
			Type: typ, Status: domain.StatusOpen, Priority: pri,
			ParentID: &plan.ID, CreatedAt: at, UpdatedAt: at,
		}
		ledger = append(ledger, domain.LedgerEntry{ID: domain.NewID(), IssueID: children[i].ID, At: at, Kind: domain.LedgerCreated})
	}

	if err := s.store.BuildPlan(plan, children, ledger); err != nil {
		return domain.Issue{}, nil, err
	}

	// Re-read so the inherited project (resolved inside the store tx) is reflected.
	builtPlan, err := s.store.GetIssue(plan.ID)
	if err != nil {
		return domain.Issue{}, nil, err
	}
	built := make([]domain.Issue, len(children))
	for i, c := range children {
		if built[i], err = s.store.GetIssue(c.ID); err != nil {
			return domain.Issue{}, nil, err
		}
	}
	return builtPlan, built, nil
}

// ApprovePlan flips a plan issue's approval from pending to approved (one-way,
// idempotent), recording it as an `updated`/`approval` ledger entry. Returns
// ErrNotFound if the issue is missing and ErrInvalidIssue if it is not a plan.
func (s *Service) ApprovePlan(id string) (domain.Issue, error) {
	entry := domain.LedgerEntry{
		ID: domain.NewID(), IssueID: id, Kind: domain.LedgerUpdated,
		Field: "approval", NewValue: string(domain.ApprovalApproved),
	}
	return s.store.ApprovePlan(id, entry)
}
