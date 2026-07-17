package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/domain"
)

// formPurpose is what submitting the form will do.
type formPurpose int

const (
	createForm formPurpose = iota
	childForm
	editForm
	commentForm
)

// field is one editable line in a form. An empty value with a placeholder means
// "leave unchanged" for edits.
type field struct {
	label       string
	value       string
	placeholder string
}

// form is the state of the active input form.
type form struct {
	purpose  formPurpose
	title    string
	fields   []field
	active   int
	targetID string // issue being edited or commented on
	parentID string // parent for a child issue
}

func (m *Model) openCreateForm() {
	m.form = form{
		purpose: createForm,
		title:   "New issue",
		fields:  []field{{label: "subject"}, {label: "body"}},
	}
	m.mode = formMode
}

func (m *Model) openChildForm(parent domain.Issue) {
	m.form = form{
		purpose:  childForm,
		title:    "New child of " + parent.ID,
		fields:   []field{{label: "subject"}, {label: "body"}},
		parentID: parent.ID,
	}
	m.mode = formMode
}

func (m *Model) openEditForm(issue domain.Issue) {
	m.form = form{
		purpose:  editForm,
		title:    "Edit " + issue.ID,
		targetID: issue.ID,
		fields: []field{
			{label: "type", placeholder: string(issue.Type)},
			{label: "status", placeholder: string(issue.Status)},
			{label: "priority", placeholder: string(issue.Priority)},
			{label: "owner", placeholder: issue.Owner},
		},
	}
	m.mode = formMode
}

func (m *Model) openCommentForm(issueID string) {
	m.form = form{
		purpose:  commentForm,
		title:    "Comment on " + issueID,
		targetID: issueID,
		fields:   []field{{label: "body"}, {label: "author"}},
	}
	m.mode = formMode
}

// handleFormKey handles keys while a form is showing.
func (m *Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = m.formReturnMode()
		return m, nil
	case tea.KeyEnter:
		return m.submitForm()
	case tea.KeyTab, tea.KeyDown:
		m.form.active = (m.form.active + 1) % len(m.form.fields)
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.form.active = (m.form.active - 1 + len(m.form.fields)) % len(m.form.fields)
		return m, nil
	case tea.KeyBackspace:
		f := &m.form.fields[m.form.active]
		if r := []rune(f.value); len(r) > 0 {
			f.value = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeySpace:
		m.form.fields[m.form.active].value += " "
		return m, nil
	case tea.KeyRunes:
		m.form.fields[m.form.active].value += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

func (m *Model) formReturnMode() viewMode {
	if m.form.purpose == commentForm && m.detail != nil {
		return detailMode
	}
	return listMode
}

func (m *Model) fieldValue(label string) string {
	for _, f := range m.form.fields {
		if f.label == label {
			return f.value
		}
	}
	return ""
}

// submitForm turns the form into the appropriate client command.
func (m *Model) submitForm() (tea.Model, tea.Cmd) {
	switch m.form.purpose {
	case createForm:
		return m, m.createIssueCmd(m.fieldValue("subject"), m.fieldValue("body"), "")
	case childForm:
		return m, m.createIssueCmd(m.fieldValue("subject"), m.fieldValue("body"), m.form.parentID)
	case editForm:
		fields := map[string]string{}
		for _, f := range m.form.fields {
			if f.value != "" {
				fields[f.label] = f.value
			}
		}
		return m, m.updateIssueCmd(m.form.targetID, fields)
	case commentForm:
		return m, m.addCommentCmd(m.form.targetID, m.fieldValue("author"), m.fieldValue("body"))
	}
	return m, nil
}

// formView renders the active form.
func (m *Model) formView() string {
	var b strings.Builder
	b.WriteString(styles.title.Render(m.form.title))
	b.WriteString("\n\n")
	for i, f := range m.form.fields {
		name := styles.fieldName.Render(f.label + ":")
		if i == m.form.active {
			name = styles.active.Render("> " + f.label + ":")
		}
		shown := f.value
		if shown == "" && f.placeholder != "" {
			shown = styles.dim.Render(f.placeholder)
		}
		b.WriteString(name + " " + shown + "\n")
	}
	b.WriteString(m.footer("tab next · enter submit · esc cancel"))
	return b.String()
}
