package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/domain"
)

// viewMode is which screen the TUI is showing.
type viewMode int

const (
	listMode viewMode = iota
	detailMode
	formMode
)

// issueDetail is an issue with the related data the detail view renders.
type issueDetail struct {
	issue       domain.Issue
	documents   []domain.Document
	transcripts []domain.Transcript
	comments    []domain.Comment
	children    []domain.Issue // tracer children, for a plan issue
	childErr    error          // set when the tracer-children load failed (best-effort)
}

// Model is the Bubble Tea model for the Sutra TUI. It talks to the daemon
// only through the client package (arch rule: tui may import domain and client
// only).
type Model struct {
	ctx    context.Context
	client *client.Client

	mode          viewMode
	width, height int

	issues []domain.Issue
	cursor int

	detail   *issueDetail
	viewport viewport.Model
	vpReady  bool

	// gen is the navigation generation. It advances on every user action that
	// starts async work; each command captures it and its response is dropped
	// if gen has since moved on, so a late response cannot overwrite the
	// current screen or clobber a newer request.
	gen int

	form       form
	submitting bool // a form command is in flight; ignore repeat submits

	statusMsg   string
	err         error
	lastCreated domain.Issue
	hasCreated  bool
	quitting    bool
}

// New builds a Model bound to the given client.
func New(ctx context.Context, c *client.Client) *Model {
	return &Model{
		ctx:    ctx,
		client: c,
		mode:   listMode,
	}
}

// Init loads the issue list.
func (m *Model) Init() tea.Cmd {
	return m.loadIssuesCmd(m.gen)
}

// Update reduces a message into new model state, returning any follow-up command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
		return m, nil

	case issuesLoadedMsg:
		if msg.gen != m.gen {
			return m, nil // stale
		}
		m.err = msg.err
		if msg.err == nil {
			m.issues = msg.issues
			if m.cursor >= len(m.issues) {
				m.cursor = max(0, len(m.issues)-1)
			}
		}
		return m, nil

	case issueCreatedMsg:
		if msg.gen != m.gen {
			return m, nil // stale
		}
		m.submitting = false
		m.err = msg.err
		if msg.err == nil {
			m.lastCreated = msg.issue
			m.hasCreated = true
			if msg.asChild {
				m.statusMsg = "created child " + msg.issue.ID
			} else {
				m.statusMsg = "created " + msg.issue.ID
			}
			m.mode = listMode
			return m, m.loadIssuesCmd(m.gen)
		}
		return m, nil

	case issueUpdatedMsg:
		if msg.gen != m.gen {
			return m, nil // stale
		}
		m.submitting = false
		m.err = msg.err
		if msg.err == nil {
			m.statusMsg = "updated " + msg.issue.ID
			m.replaceIssue(msg.issue) // keep the list cache current
			// Return to where the edit was opened from. Editing from the detail
			// view refreshes the detail; editing from the list refreshes the list.
			if m.form.origin == detailMode && m.detail != nil && m.detail.issue.ID == msg.issue.ID {
				m.mode = detailMode
				return m, m.loadDetailCmd(msg.issue.ID, m.gen)
			}
			m.mode = listMode
			return m, m.loadIssuesCmd(m.gen)
		}
		return m, nil

	case detailLoadedMsg:
		if msg.gen != m.gen {
			return m, nil // stale: the user has navigated since this load began
		}
		m.err = msg.err
		if msg.err == nil {
			d := msg.detail
			m.detail = &d
			m.mode = detailMode
			m.setDetailContent()
			if m.vpReady {
				m.viewport.GotoTop() // start a freshly opened issue at the top
			}
		}
		return m, nil

	case commentAddedMsg:
		if msg.gen != m.gen {
			return m, nil // stale
		}
		m.submitting = false
		m.err = msg.err
		if msg.err == nil {
			id := msg.comment.IssueID
			m.statusMsg = "commented on " + id
			if m.detail != nil && m.detail.issue.ID == id {
				m.mode = detailMode
				return m, m.loadDetailCmd(id, m.gen) // re-fetch so the new comment shows
			}
		}
		return m, nil

	case planApprovedMsg:
		if msg.gen != m.gen {
			return m, nil // stale
		}
		m.err = msg.err
		if msg.err == nil {
			m.statusMsg = "approved " + msg.issue.ID
			// Apply the returned issue immediately so the detail reflects the new
			// approval without depending on a follow-up reload that could fail and
			// leave a stale "pending" that still offers approval. Approval does not
			// change the tracer children, so no re-fetch is needed.
			if m.detail != nil && m.detail.issue.ID == msg.issue.ID {
				m.detail.issue = msg.issue
				m.setDetailContent()
			}
			m.replaceIssue(msg.issue) // keep the list cache current
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		m.quitting = true
		return m, tea.Quit
	}
	switch m.mode {
	case listMode:
		return m.handleListKey(msg)
	case detailMode:
		return m.handleDetailKey(msg)
	case formMode:
		return m.handleFormKey(msg)
	}
	return m, nil
}

// View renders the current screen.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	switch m.mode {
	case detailMode:
		return m.detailView()
	case formMode:
		return m.formView()
	default:
		return m.listView()
	}
}

// replaceIssue updates the cached list entry for issue in place, so the list
// reflects an edit immediately without waiting for a reload.
func (m *Model) replaceIssue(issue domain.Issue) {
	for i := range m.issues {
		if m.issues[i].ID == issue.ID {
			m.issues[i] = issue
			return
		}
	}
}

// bumpGen advances and returns the navigation generation. Any in-flight command
// tagged with an older generation is dropped when its response arrives, so
// navigating away cannot be clobbered by a late response.
func (m *Model) bumpGen() int {
	m.gen++
	return m.gen
}

func (m *Model) resizeViewport() {
	// Reserve rows for the footer, which is up to 3 lines (blank + status/error
	// + help) when a status or error message is present.
	h := m.height - footerRows
	if h < 1 {
		h = 1
	}
	w := m.width
	if w < 1 {
		w = 1
	}
	if !m.vpReady {
		m.viewport = viewport.New(w, h)
		m.vpReady = true
	} else {
		m.viewport.Width, m.viewport.Height = w, h
	}
	m.setDetailContent()
}

// --- exported accessors for tests and callers ---

// ModeName returns the active screen: "list", "detail", or "form".
func (m *Model) ModeName() string {
	switch m.mode {
	case detailMode:
		return "detail"
	case formMode:
		return "form"
	default:
		return "list"
	}
}

// Issues returns the currently loaded issue list.
func (m *Model) Issues() []domain.Issue { return m.issues }

// SelectedIssue returns the issue under the list cursor, if any.
func (m *Model) SelectedIssue() (domain.Issue, bool) {
	if m.cursor < 0 || m.cursor >= len(m.issues) {
		return domain.Issue{}, false
	}
	return m.issues[m.cursor], true
}

// LastCreated reports the most recently created issue, if any.
func (m *Model) LastCreated() (domain.Issue, bool) { return m.lastCreated, m.hasCreated }

// LastError returns the most recent client error, if any.
func (m *Model) LastError() error { return m.err }

// DetailIssue returns the issue currently shown in the detail view.
func (m *Model) DetailIssue() (domain.Issue, bool) {
	if m.detail == nil {
		return domain.Issue{}, false
	}
	return m.detail.issue, true
}

// DetailDocuments returns the documents shown in the detail view.
func (m *Model) DetailDocuments() []domain.Document {
	if m.detail == nil {
		return nil
	}
	return m.detail.documents
}

// DetailTranscripts returns the linked transcripts shown in the detail view.
func (m *Model) DetailTranscripts() []domain.Transcript {
	if m.detail == nil {
		return nil
	}
	return m.detail.transcripts
}

// DetailComments returns the comments shown in the detail view.
func (m *Model) DetailComments() []domain.Comment {
	if m.detail == nil {
		return nil
	}
	return m.detail.comments
}

// DetailChildren returns the tracer children shown for a plan issue's detail.
func (m *Model) DetailChildren() []domain.Issue {
	if m.detail == nil {
		return nil
	}
	return m.detail.children
}
