package tui

// White-box unit tests for the model behaviours that the synchronous BDD
// scenario does not exercise: control-sequence sanitization, the
// duplicate-submit guard, explicit owner clearing, and stale-response
// handling. These need no daemon.

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/domain"
)

func TestCleanStripsControlSequences(t *testing.T) {
	// ESC-based ANSI, a raw CR, and a DEL must not survive; text does.
	in := "hello\x1b[31mRED\x1b[0m\x07\x7fworld"
	got := clean(in)
	if strings.ContainsAny(got, "\x1b\x07\x7f") {
		t.Fatalf("clean left control chars: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") || !strings.Contains(got, "RED") {
		t.Fatalf("clean dropped visible text: %q", got)
	}
	// clean preserves newlines/tabs; cleanLine folds them to spaces.
	if clean("a\nb\tc") != "a\nb\tc" {
		t.Fatalf("clean should preserve newlines/tabs: %q", clean("a\nb\tc"))
	}
	if strings.ContainsAny(cleanLine("a\nb\tc"), "\n\t") {
		t.Fatalf("cleanLine should fold newlines/tabs: %q", cleanLine("a\nb\tc"))
	}
}

func TestDuplicateSubmitGuard(t *testing.T) {
	m := New(context.Background(), nil)
	m.openCreateForm()
	m.form.fields[0].value = "subj"
	m.form.fields[1].value = "body"

	// First Enter submits and returns a command.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("first submit returned no command")
	}
	if !m.submitting {
		t.Fatal("submitting flag not set after first submit")
	}
	// A second Enter while the command is in flight must be ignored.
	_, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd2 != nil {
		t.Fatal("second submit issued a command while one was in flight")
	}
	// The result message clears the guard.
	m.Update(issueCreatedMsg{issue: domain.Issue{ID: "x"}})
	if m.submitting {
		t.Fatal("submitting flag not cleared after the command completed")
	}
}

func TestEditFieldsOwnerClearAndUntouched(t *testing.T) {
	m := New(context.Background(), nil)
	m.openEditForm(domain.Issue{ID: "i1", Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2, Owner: "old"})

	// Move to the owner field (index 3) and clear it with a backspace.
	m.form.active = 3
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	fields := m.editFields()
	if v, ok := fields["owner"]; !ok || v != "" {
		t.Fatalf("owner clear not captured: %v", fields)
	}
	// Untouched enum fields are not sent.
	for _, k := range []string{"type", "status", "priority"} {
		if _, ok := fields[k]; ok {
			t.Fatalf("untouched field %q should not be sent: %v", k, fields)
		}
	}

	// An edited-but-empty enum is left unchanged (no valid empty value).
	m.openEditForm(domain.Issue{ID: "i2", Status: domain.StatusOpen})
	m.form.active = 1 // status
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if _, ok := m.editFields()["status"]; ok {
		t.Fatal("edited-but-empty status should not be sent")
	}
}

func TestStaleDetailResponseIgnored(t *testing.T) {
	m := New(context.Background(), nil)
	// Simulate the user having navigated: the current generation is 2, but a
	// response tagged with generation 1 arrives late.
	m.detailSeq = 2
	m.openCreateForm() // bumps to 3 and enters form mode
	before := m.mode
	m.Update(detailLoadedMsg{seq: 1, detail: issueDetail{issue: domain.Issue{ID: "stale"}}})
	if m.detail != nil {
		t.Fatal("stale detail response was applied")
	}
	if m.mode != before {
		t.Fatalf("stale response changed mode: %v -> %v", before, m.mode)
	}
}
