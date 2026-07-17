package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleListKey handles keys while the issue list is showing.
func (m *Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.issues)-1 {
			m.cursor++
		}
	case "r":
		return m, m.loadIssuesCmd(m.bumpGen())
	case "enter":
		if len(m.issues) > 0 {
			return m, m.loadDetailCmd(m.issues[m.cursor].ID, m.bumpGen())
		}
	case "n":
		m.openCreateForm()
	case "c":
		if len(m.issues) > 0 {
			m.openChildForm(m.issues[m.cursor])
		}
	case "e":
		if len(m.issues) > 0 {
			m.openEditForm(m.issues[m.cursor])
		}
	}
	return m, nil
}

// listView renders the issue list.
func (m *Model) listView() string {
	var b strings.Builder
	b.WriteString(styles.title.Render(truncate("Sutra — issues", m.width)))
	b.WriteByte('\n')
	if len(m.issues) == 0 {
		b.WriteString(styles.dim.Render(truncate("(no issues) — press n to create one", m.width)))
		b.WriteByte('\n')
	}
	// Render a height-bounded window around the cursor so the selected row stays
	// on screen on a short terminal.
	start, end := m.listWindow()
	if start > 0 {
		b.WriteString(styles.dim.Render(truncate(fmt.Sprintf("  ↑ %d more", start), m.width)) + "\n")
	}
	for i := start; i < end; i++ {
		is := m.issues[i]
		// Build the plain line with fixed spacing (no tabs — tab display width is
		// ambiguous), then truncate it to the terminal width (minus the 2-col
		// cursor) so a long subject can't wrap onto a second row and push the
		// selection off-screen, THEN apply styling.
		line := fmt.Sprintf("%s  [%s/%s/%s]  %s", cleanLine(is.ID), is.Type, is.Status, is.Priority, cleanLine(is.Subject))
		if is.ParentID != nil {
			line += " (child of " + cleanLine(*is.ParentID) + ")"
		}
		if m.width > 0 {
			// The 2-cell cursor is always rendered; when it consumes all the
			// width, the issue text collapses to empty rather than forcing a wrap.
			if avail := m.width - 2; avail <= 0 {
				line = ""
			} else {
				line = truncate(line, avail)
			}
		}
		cursor := "  "
		if i == m.cursor {
			cursor = styles.cursor.Render("> ")
			line = styles.selected.Render(line)
		}
		b.WriteString(cursor + line + "\n")
	}
	if end < len(m.issues) {
		b.WriteString(styles.dim.Render(truncate(fmt.Sprintf("  ↓ %d more", len(m.issues)-end), m.width)) + "\n")
	}
	b.WriteString(m.footer("↑/↓ move · enter open · n new · c child · e edit · r refresh · q quit"))
	return b.String()
}

// footerRows is the maximum height of the footer: a blank line, an optional
// status/error line, and the help line.
const footerRows = 3

// listWindow returns the [start,end) range of issues to render so the cursor is
// always visible within the terminal height. With no known height (e.g. before
// the first WindowSizeMsg), it renders the whole list.
func (m *Model) listWindow() (start, end int) {
	n := len(m.issues)
	// Reserve rows for the title (1), the up/down "more" indicators (2), and the
	// footer (footerRows).
	rows := m.height - footerRows - 3
	if m.height <= 0 || rows >= n {
		return 0, n
	}
	if rows < 1 {
		rows = 1
	}
	start = m.cursor - rows/2
	if start < 0 {
		start = 0
	}
	end = start + rows
	if end > n {
		end = n
		start = end - rows
	}
	if start < 0 {
		start = 0
	}
	return start, end
}

// footer renders the status/error line plus a help hint.
func (m *Model) footer(help string) string {
	var b strings.Builder
	b.WriteByte('\n')
	if m.err != nil {
		b.WriteString(styles.errMsg.Render(truncate("error: "+cleanLine(m.err.Error()), m.width)) + "\n")
	} else if m.statusMsg != "" {
		b.WriteString(styles.status.Render(truncate(cleanLine(m.statusMsg), m.width)) + "\n")
	}
	b.WriteString(styles.help.Render(truncate(help, m.width)))
	return b.String()
}
