package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/domain"
)

func planDetailModel(typ domain.IssueType, approval domain.Approval) *Model {
	m := New(context.Background(), nil)
	m.mode = detailMode
	m.detail = &issueDetail{issue: domain.Issue{
		ID: "p1", Subject: "The plan", Type: typ,
		Status: domain.StatusOpen, Priority: domain.P2, Approval: approval,
	}}
	return m
}

func pressA(m *Model) tea.Cmd {
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	return cmd
}

func TestApproveKeyOnPendingPlanIssuesCommand(t *testing.T) {
	m := planDetailModel(domain.TypePlan, domain.ApprovalPending)
	g0 := m.gen
	if cmd := pressA(m); cmd == nil {
		t.Fatal("expected an approve command for a pending plan")
	}
	if m.gen == g0 {
		t.Fatal("expected gen to advance when approving")
	}
}

func TestApproveKeyIsNoopForNonPendingOrNonPlan(t *testing.T) {
	// Already-approved plan: no command, but a status message.
	m := planDetailModel(domain.TypePlan, domain.ApprovalApproved)
	if cmd := pressA(m); cmd != nil {
		t.Fatal("expected no command for an already-approved plan")
	}
	if m.statusMsg == "" {
		t.Fatal("expected a status message explaining the no-op")
	}
	// Non-plan issue: no command.
	m2 := planDetailModel(domain.TypeTask, "")
	if cmd := pressA(m2); cmd != nil {
		t.Fatal("expected no command for a non-plan issue")
	}
}

func TestPlanApprovedMsgReloadsDetailAndSetsStatus(t *testing.T) {
	m := planDetailModel(domain.TypePlan, domain.ApprovalPending)
	_, cmd := m.Update(planApprovedMsg{gen: m.gen, issue: domain.Issue{ID: "p1", Type: domain.TypePlan, Approval: domain.ApprovalApproved}})
	if cmd == nil {
		t.Fatal("expected a detail-reload command after approval")
	}
	if m.statusMsg != "approved p1" {
		t.Errorf("status = %q, want %q", m.statusMsg, "approved p1")
	}
	if m.err != nil {
		t.Errorf("unexpected err: %v", m.err)
	}
}

func TestPlanApprovedMsgStaleIsDropped(t *testing.T) {
	m := planDetailModel(domain.TypePlan, domain.ApprovalPending)
	if _, cmd := m.Update(planApprovedMsg{gen: m.gen + 1, issue: domain.Issue{ID: "p1"}}); cmd != nil {
		t.Fatal("a stale approval message must be dropped")
	}
}

func TestRenderDetailShowsTracers(t *testing.T) {
	m := planDetailModel(domain.TypePlan, domain.ApprovalApproved)
	m.detail.children = []domain.Issue{
		{ID: "c1", Subject: "first tracer", Status: domain.StatusClosed},
		{ID: "c2", Subject: "second tracer", Status: domain.StatusOpen},
	}
	out := m.renderDetail()
	for _, want := range []string{"Tracers (2)", "c1", "first tracer", "closed", "c2", "second tracer"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan detail missing %q:\n%s", want, out)
		}
	}
	// A non-plan issue shows no Tracers section.
	if strings.Contains(planDetailModel(domain.TypeTask, "").renderDetail(), "Tracers") {
		t.Error("non-plan detail should not show a Tracers section")
	}
}

func TestRenderDetailShowsPlanApproval(t *testing.T) {
	m := planDetailModel(domain.TypePlan, domain.ApprovalPending)
	out := m.renderDetail()
	if !strings.Contains(out, "approval") || !strings.Contains(out, "pending") {
		t.Errorf("plan detail missing approval line:\n%s", out)
	}
	// A non-plan issue shows no approval line.
	m2 := planDetailModel(domain.TypeTask, "")
	if strings.Contains(m2.renderDetail(), "approval") {
		t.Errorf("non-plan detail should not show approval:\n%s", m2.renderDetail())
	}
}
