package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/domain"
)

// The TUI reaches the daemon exclusively through the client package. Each
// interaction is a tea.Cmd that performs one client call off the UI thread and
// reports its outcome back as a message the model reduces in Update.
//
// Every command carries a generation (gen): the model's navigation counter at
// the time the command was issued. When the response arrives the model drops it
// if gen no longer matches the current generation — i.e. the user has navigated
// since — so a late response cannot overwrite the current screen or clobber a
// newer request.
//
// ACCEPTED LIMITATION (out of scope for Slice 8): create and comment
// submissions are not end-to-end idempotent. If the daemon commits but the
// response is lost or times out, the model surfaces an error and a retry can
// create a duplicate. True idempotency requires a per-submission key that the
// server deduplicates against a persisted store — a cross-cutting reliability
// feature (client + api + service + store schema) that belongs with the
// networking/LAN track (Slice 2), not the TUI. Documented rather than
// half-implemented client-side, which would not actually deduplicate.

type issuesLoadedMsg struct {
	gen    int
	issues []domain.Issue
	err    error
}

type issueCreatedMsg struct {
	gen     int
	issue   domain.Issue
	asChild bool
	err     error
}

type issueUpdatedMsg struct {
	gen   int
	issue domain.Issue
	err   error
}

type detailLoadedMsg struct {
	gen    int
	detail issueDetail
	err    error
}

type commentAddedMsg struct {
	gen     int
	comment domain.Comment
	err     error
}

type planApprovedMsg struct {
	gen   int
	issue domain.Issue
	err   error
}

// approvePlanCmd approves a plan issue via the daemon.
func (m *Model) approvePlanCmd(id string, gen int) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.ApprovePlan(ctx, id)
		return planApprovedMsg{gen: gen, issue: res.Issue, err: err}
	}
}

// loadIssuesCmd lists live issues.
func (m *Model) loadIssuesCmd(gen int) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.ListIssues(ctx, nil)
		return issuesLoadedMsg{gen: gen, issues: res.Issues, err: err}
	}
}

// createIssueCmd creates an issue. When parentID is set it creates a child in a
// single atomic request, so a failure leaves no orphan and a retry cannot
// duplicate the issue.
func (m *Model) createIssueCmd(subject, body, parentID string, gen int) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		if parentID != "" {
			res, err := c.CreateChildIssue(ctx, subject, body, parentID)
			return issueCreatedMsg{gen: gen, issue: res.Issue, asChild: true, err: err}
		}
		res, err := c.CreateIssue(ctx, subject, body)
		return issueCreatedMsg{gen: gen, issue: res.Issue, err: err}
	}
}

// updateIssueCmd changes the given fields on an issue.
func (m *Model) updateIssueCmd(id string, fields map[string]string, gen int) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.UpdateIssue(ctx, id, fields)
		return issueUpdatedMsg{gen: gen, issue: res.Issue, err: err}
	}
}

// loadDetailCmd fetches an issue with its documents, comments, and linked
// transcripts.
func (m *Model) loadDetailCmd(id string, gen int) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		ir, err := c.GetIssue(ctx, id)
		if err != nil {
			return detailLoadedMsg{gen: gen, err: err}
		}
		d := issueDetail{issue: ir.Issue}
		dr, err := c.ListDocuments(ctx, id)
		if err != nil {
			return detailLoadedMsg{gen: gen, err: err}
		}
		d.documents = dr.Documents
		cr, err := c.ListComments(ctx, id)
		if err != nil {
			return detailLoadedMsg{gen: gen, err: err}
		}
		d.comments = cr.Comments
		tr, err := c.TranscriptsForIssue(ctx, id)
		if err != nil {
			return detailLoadedMsg{gen: gen, err: err}
		}
		d.transcripts = tr.Transcripts
		// A plan issue also shows its tracer children, in run order. Loading them
		// is best-effort: a child-list failure must not discard the successfully
		// loaded detail (which would, on the post-approval path, drop the whole
		// issue). The tracer section simply renders empty if the fetch fails.
		if ir.Issue.Type == domain.TypePlan {
			if lr, err := c.ListIssues(ctx, map[string]string{"parent": id}); err == nil {
				d.children = lr.Issues
			} else {
				d.childErr = err // recorded so the tracer section renders "unavailable", not empty
			}
		}
		return detailLoadedMsg{gen: gen, detail: d}
	}
}

// addCommentCmd posts a comment to an issue.
func (m *Model) addCommentCmd(id, author, body string, gen int) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.AddComment(ctx, id, author, body)
		return commentAddedMsg{gen: gen, comment: res.Comment, err: err}
	}
}
