package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/domain"
)

// handleDetailKey handles keys while an issue's detail is showing.
func (m *Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.bumpGen()    // invalidate any in-flight load
		m.detail = nil // discard so a later action can't follow a stale detail
		m.mode = listMode
		return m, nil
	case "a":
		if m.detail != nil {
			m.openCommentForm(m.detail.issue.ID)
		}
		return m, nil
	case "A":
		if m.detail != nil && m.detail.issue.Type == domain.TypePlan && m.detail.issue.Approval == domain.ApprovalPending {
			gen := m.bumpGen()
			return m, m.approvePlanCmd(m.detail.issue.ID, gen)
		}
		if m.detail != nil {
			m.statusMsg = "only a pending plan can be approved"
		}
		return m, nil
	default:
		if m.vpReady {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

// setDetailContent renders the detail into the scrollable viewport.
func (m *Model) setDetailContent() {
	if !m.vpReady || m.detail == nil {
		return
	}
	m.viewport.SetContent(m.renderDetail())
}

// detailView renders the detail screen.
func (m *Model) detailView() string {
	if m.detail == nil {
		return "loading…"
	}
	body := m.renderDetail()
	if m.vpReady {
		body = m.viewport.View()
	}
	hint := "↑/↓ scroll · a comment · esc back · q back"
	if m.detail != nil && m.detail.issue.Type == domain.TypePlan && m.detail.issue.Approval == domain.ApprovalPending {
		hint = "↑/↓ scroll · a comment · A approve · esc back · q back"
	}
	return body + m.footer(hint)
}

// renderDetail builds the detail text: core fields plus documents, comments,
// and linked transcripts.
func (m *Model) renderDetail() string {
	d := m.detail
	i := d.issue
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", styles.title.Render("Issue "+cleanLine(i.ID)))
	fmt.Fprintf(&b, "%s %s\n", styles.fieldName.Render("subject:"), cleanLine(i.Subject))
	fmt.Fprintf(&b, "%s %s\n", styles.fieldName.Render("type:"), i.Type)
	fmt.Fprintf(&b, "%s %s\n", styles.fieldName.Render("status:"), i.Status)
	fmt.Fprintf(&b, "%s %s\n", styles.fieldName.Render("priority:"), i.Priority)
	if i.Type == domain.TypePlan && i.Approval != "" {
		fmt.Fprintf(&b, "%s %s\n", styles.fieldName.Render("approval:"), i.Approval)
	}
	if i.Owner != "" {
		fmt.Fprintf(&b, "%s %s\n", styles.fieldName.Render("owner:"), cleanLine(i.Owner))
	}
	if i.ParentID != nil {
		fmt.Fprintf(&b, "%s %s\n", styles.fieldName.Render("parent:"), cleanLine(*i.ParentID))
	}
	fmt.Fprintf(&b, "\n%s\n", clean(i.Body))

	if i.Type == domain.TypePlan {
		if d.childErr != nil {
			// Distinguish a load failure from a genuinely childless plan.
			fmt.Fprintf(&b, "\n%s\n", styles.section.Render("Tracers (unavailable)"))
			b.WriteString(styles.dim.Render("  could not load tracers: "+cleanLine(d.childErr.Error())) + "\n")
		} else {
			fmt.Fprintf(&b, "\n%s\n", styles.section.Render(fmt.Sprintf("Tracers (%d)", len(d.children))))
			if len(d.children) == 0 {
				b.WriteString(styles.dim.Render("  (none)") + "\n")
			}
			for idx, c := range d.children {
				fmt.Fprintf(&b, "  %d. %s [%s] %s\n", idx+1, cleanLine(c.ID), c.Status, cleanLine(c.Subject))
			}
		}
	}

	fmt.Fprintf(&b, "\n%s\n", styles.section.Render(fmt.Sprintf("Documents (%d)", len(d.documents))))
	if len(d.documents) == 0 {
		b.WriteString(styles.dim.Render("  (none)") + "\n")
	}
	for _, doc := range d.documents {
		fmt.Fprintf(&b, "  - [%s] %s\n", doc.Kind, cleanLine(doc.Title))
	}

	fmt.Fprintf(&b, "\n%s\n", styles.section.Render(fmt.Sprintf("Comments (%d)", len(d.comments))))
	if len(d.comments) == 0 {
		b.WriteString(styles.dim.Render("  (none)") + "\n")
	}
	for _, c := range d.comments {
		author := c.Author
		if author == "" {
			author = "anon"
		}
		fmt.Fprintf(&b, "  - %s: %s\n", cleanLine(author), cleanLine(c.Body))
	}

	fmt.Fprintf(&b, "\n%s\n", styles.section.Render(fmt.Sprintf("Linked transcripts (%d)", len(d.transcripts))))
	if len(d.transcripts) == 0 {
		b.WriteString(styles.dim.Render("  (none)") + "\n")
	}
	for _, t := range d.transcripts {
		title := t.Title
		if title == "" {
			title = t.SessionID
		}
		fmt.Fprintf(&b, "  - %s (%s)\n", cleanLine(title), t.CapturedAt.Format("2006-01-02"))
	}
	return b.String()
}
