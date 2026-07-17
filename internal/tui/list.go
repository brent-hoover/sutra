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
	b.WriteString(styles.title.Render("Sutra — issues"))
	b.WriteByte('\n')
	if len(m.issues) == 0 {
		b.WriteString(styles.dim.Render("(no issues) — press n to create one"))
		b.WriteByte('\n')
	}
	for i, is := range m.issues {
		cursor := "  "
		line := fmt.Sprintf("%s\t[%s/%s/%s]\t%s", cleanLine(is.ID), is.Type, is.Status, is.Priority, cleanLine(is.Subject))
		if is.ParentID != nil {
			line += styles.dim.Render(" (child of " + cleanLine(*is.ParentID) + ")")
		}
		if i == m.cursor {
			cursor = styles.cursor.Render("> ")
			line = styles.selected.Render(line)
		}
		b.WriteString(cursor + line + "\n")
	}
	b.WriteString(m.footer("↑/↓ move · enter open · n new · c child · e edit · r refresh · q quit"))
	return b.String()
}

// footer renders the status/error line plus a help hint.
func (m *Model) footer(help string) string {
	var b strings.Builder
	b.WriteByte('\n')
	if m.err != nil {
		b.WriteString(styles.errMsg.Render("error: "+cleanLine(m.err.Error())) + "\n")
	} else if m.statusMsg != "" {
		b.WriteString(styles.status.Render(cleanLine(m.statusMsg)) + "\n")
	}
	b.WriteString(styles.help.Render(help))
	return b.String()
}
