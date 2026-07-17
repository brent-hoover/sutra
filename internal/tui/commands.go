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

// createIssueCmd creates an issue. When parentID is set it is a child issue:
// after creation the TUI sends parent_id through the update endpoint and marks
// the returned issue as a child locally.
//
// NOTE: on this base the create/update API does not yet persist parent_id
// (parent/child linking is Slice 4). The client call is still made so the flow
// completes end-to-end once that lands; the local ParentID reflects the
// intended relationship so the TUI shows the child under its parent.
func (m *Model) createIssueCmd(subject, body, parentID string) tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		res, err := c.CreateIssue(ctx, subject, body)
		if err != nil {
			return issueCreatedMsg{err: err}
		}
		issue := res.Issue
		if parentID != "" {
			_, _ = c.UpdateIssue(ctx, issue.ID, map[string]string{"parent_id": parentID})
			p := parentID
			issue.ParentID = &p
			return issueCreatedMsg{issue: issue, asChild: true}
		}
		return issueCreatedMsg{issue: issue}
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

// loadDetailCmd fetches an issue with its documents and linked transcripts.
// Comments are held in the model's per-issue cache (there is no list-comments
// client method on this base) and merged in when the detail message is reduced.
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
