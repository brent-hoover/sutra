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

// field is one editable line in a form. For edits, edited distinguishes an
// untouched field ("leave unchanged") from one deliberately set empty (e.g.
// clearing owner), which value alone cannot express.
type field struct {
	label       string
	value       string
	placeholder string
	edited      bool
}

// form is the state of the active input form.
type form struct {
	purpose  formPurpose
	title    string
	fields   []field
	active   int
	targetID string   // issue being edited or commented on
	parentID string   // parent for a child issue
	origin   viewMode // screen to return to on cancel/success
}

func (m *Model) openCreateForm() {
	m.form = form{
		purpose: createForm,
		title:   "New issue",
		fields:  []field{{label: "subject"}, {label: "body"}},
		origin:  m.mode,
	}
	m.bumpGen() // invalidate any in-flight load
	m.mode = formMode
}

func (m *Model) openChildForm(parent domain.Issue) {
	m.form = form{
		purpose:  childForm,
		title:    "New child of " + parent.ID,
		fields:   []field{{label: "subject"}, {label: "body"}},
		parentID: parent.ID,
		origin:   m.mode,
	}
	m.bumpGen() // invalidate any in-flight load
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
		origin: m.mode,
	}
	m.bumpGen() // invalidate any in-flight load
	m.mode = formMode
}

func (m *Model) openCommentForm(issueID string) {
	m.form = form{
		purpose:  commentForm,
		title:    "Comment on " + issueID,
		targetID: issueID,
		fields:   []field{{label: "body"}, {label: "author"}},
		origin:   m.mode,
	}
	m.bumpGen() // invalidate any in-flight load
	m.mode = formMode
}

// handleFormKey handles keys while a form is showing.
func (m *Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.submitting {
			// A submit is in flight: the HTTP request cannot be un-sent, so we
			// must not let the user "cancel" and risk a duplicate on retry.
			// Stay until the response arrives (or the client times out).
			return m, nil
		}
		m.bumpGen()
		m.mode = m.formReturnMode()
		return m, nil
	case tea.KeyEnter:
		if m.submitting {
			return m, nil // a submit is already in flight; ignore repeats
		}
		return m.submitForm()
	case tea.KeyTab, tea.KeyDown:
		m.form.active = (m.form.active + 1) % len(m.form.fields)
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.form.active = (m.form.active - 1 + len(m.form.fields)) % len(m.form.fields)
		return m, nil
	case tea.KeyBackspace:
		f := &m.form.fields[m.form.active]
		f.edited = true
		if r := []rune(f.value); len(r) > 0 {
			f.value = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeySpace:
		f := &m.form.fields[m.form.active]
		f.value += " "
		f.edited = true
		return m, nil
	case tea.KeyRunes:
		f := &m.form.fields[m.form.active]
		f.value += string(msg.Runes)
		f.edited = true
		return m, nil
	}
	return m, nil
}

func (m *Model) formReturnMode() viewMode {
	if m.form.origin == detailMode && m.detail != nil {
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

// submitForm turns the form into the appropriate client command. The command
// is tagged with a fresh generation so navigating away before it returns drops
// its result instead of letting it land on another screen.
func (m *Model) submitForm() (tea.Model, tea.Cmd) {
	m.submitting = true
	g := m.bumpGen()
	switch m.form.purpose {
	case createForm:
		return m, m.createIssueCmd(m.fieldValue("subject"), m.fieldValue("body"), "", g)
	case childForm:
		return m, m.createIssueCmd(m.fieldValue("subject"), m.fieldValue("body"), m.form.parentID, g)
	case editForm:
		return m, m.updateIssueCmd(m.form.targetID, m.editFields(), g)
	case commentForm:
		return m, m.addCommentCmd(m.form.targetID, m.fieldValue("author"), m.fieldValue("body"), g)
	}
	return m, nil
}

// editFields collects the changes from an edit form. An untouched field is
// left unchanged; owner is free text and may be cleared (edited to empty),
// while the enum fields have no valid empty value so an edited-but-empty enum
// is left unchanged.
func (m *Model) editFields() map[string]string {
	fields := map[string]string{}
	for _, f := range m.form.fields {
		if !f.edited {
			continue
		}
		if f.label == "owner" || f.value != "" {
			fields[f.label] = f.value
		}
	}
	return fields
}

// formView renders the active form.
func (m *Model) formView() string {
	var b strings.Builder
	b.WriteString(styles.title.Render(truncate(m.form.title, m.width)))
	b.WriteString("\n\n")
	for i, f := range m.form.fields {
		name := styles.fieldName.Render(f.label + ":")
		if i == m.form.active {
			name = styles.active.Render("> " + f.label + ":")
		}
		shown := cleanLine(f.value)
		// Show the placeholder (current value) when the field is empty and its
		// current value will be preserved on submit. Only `owner` submits an
		// empty value (clearing it); enum fields omit an empty value (keeping
		// the current one), so their placeholder stays visible even when edited
		// — the display then never contradicts what submission does.
		if f.value == "" && f.placeholder != "" && !(f.edited && f.label == "owner") {
			shown = styles.dim.Render(cleanLine(f.placeholder))
		}
		// Truncate the composed (styled) field row to the terminal width so a
		// long value can't wrap and push later fields / the help off-screen.
		b.WriteString(truncate(name+" "+shown, m.width) + "\n")
	}
	b.WriteString(m.footer("tab next · enter submit · esc cancel"))
	return b.String()
}
