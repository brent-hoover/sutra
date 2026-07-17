package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/domain"
)

// The TUI reaches the daemon exclusively through the client package. Each
// interaction is a tea.Cmd that performs one client call off the UI thread and
// reports its outcome back as a message the model reduces in Update.

type issuesLoadedMsg struct {
	issues []domain.Issue
	err    error
}

type issueCreatedMsg struct {
	issue   domain.Issue
	asChild bool
	err     error
}

type issueUpdatedMsg struct {
	issue domain.Issue
	err   error
}

type detailLoadedMsg struct {
	detail issueDetail
	err    error
}

type commentAddedMsg struct {
	comment domain.Comment
	err     error
}

// loadIssuesCmd lists live issues.
func (m *Model) loadIssuesCmd() tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.ListIssues(ctx, nil)
		return issuesLoadedMsg{issues: res.Issues, err: err}
	}
}

// createIssueCmd creates an issue. When parentID is set it creates a child in a
// single atomic request, so a failure leaves no orphan and a retry cannot
// duplicate the issue.
func (m *Model) createIssueCmd(subject, body, parentID string) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		if parentID != "" {
			res, err := c.CreateChildIssue(ctx, subject, body, parentID)
			return issueCreatedMsg{issue: res.Issue, asChild: true, err: err}
		}
		res, err := c.CreateIssue(ctx, subject, body)
		return issueCreatedMsg{issue: res.Issue, err: err}
	}
}

// updateIssueCmd changes the given fields on an issue.
func (m *Model) updateIssueCmd(id string, fields map[string]string) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.UpdateIssue(ctx, id, fields)
		return issueUpdatedMsg{issue: res.Issue, err: err}
	}
}

// loadDetailCmd fetches an issue with its documents, comments, and linked
// transcripts.
func (m *Model) loadDetailCmd(id string) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		ir, err := c.GetIssue(ctx, id)
		if err != nil {
			return detailLoadedMsg{err: err}
		}
		d := issueDetail{issue: ir.Issue}
		dr, err := c.ListDocuments(ctx, id)
		if err != nil {
			return detailLoadedMsg{err: err}
		}
		d.documents = dr.Documents
		cr, err := c.ListComments(ctx, id)
		if err != nil {
			return detailLoadedMsg{err: err}
		}
		d.comments = cr.Comments
		tr, err := c.TranscriptsForIssue(ctx, id)
		if err != nil {
			return detailLoadedMsg{err: err}
		}
		d.transcripts = tr.Transcripts
		return detailLoadedMsg{detail: d}
	}
}

// addCommentCmd posts a comment to an issue.
func (m *Model) addCommentCmd(id, author, body string) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.AddComment(ctx, id, author, body)
		return commentAddedMsg{comment: res.Comment, err: err}
	}
}
