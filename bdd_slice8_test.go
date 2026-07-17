package main_test

// Step definitions for the @slice8 scenario "Manage issues in the TUI".
//
// The godog world drives real CLI commands and cannot interactively drive a
// Bubble Tea program, so these steps drive the tea.Model directly: they build
// the model pointed at the running daemon (via the world's client/config), feed
// it tea.KeyMsg / tea.Msg values through Update, synchronously drain the
// resulting commands (which perform the real client calls), and assert on model
// state, View() output, and persisted state through the world's verify handle.
//
// The TUI reaches the daemon only through the client package (arch rule:
// tui may import domain and client only).

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/tui"
)

// registerSlice8Steps wires the TUI scenario. It keeps its own per-scenario
// state in a closure (the ScenarioInitializer runs per scenario) so it need not
// touch the shared world struct.
func registerSlice8Steps(sc *godog.ScenarioContext, w *world) {
	type s8 struct {
		model    *tui.Model
		child    domain.Issue
		hasChild bool
	}
	st := &s8{}

	// drain runs commands until the model settles, feeding each command's
	// message back into Update. Commands perform the real client calls.
	drain := func(cmd tea.Cmd) {
		for cmd != nil {
			msg := cmd()
			if msg == nil {
				return
			}
			mdl, next := st.model.Update(msg)
			st.model = mdl.(*tui.Model)
			cmd = next
		}
	}
	send := func(msg tea.Msg) {
		mdl, cmd := st.model.Update(msg)
		st.model = mdl.(*tui.Model)
		drain(cmd)
	}
	typeStr := func(s string) { send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	press := func(t tea.KeyType) { send(tea.KeyMsg{Type: t}) }
	cmdKey := func(r rune) { send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }

	ensureList := func() {
		for i := 0; i < 3 && st.model.ModeName() != "list"; i++ {
			press(tea.KeyEsc)
		}
		cmdKey('r') // refresh the list from the daemon
	}
	selectIssue := func(id string) error {
		ensureList()
		n := len(st.model.Issues())
		for i := 0; i < n; i++ {
			press(tea.KeyUp)
		}
		for i := 0; i < n; i++ {
			if s, ok := st.model.SelectedIssue(); ok && s.ID == id {
				return nil
			}
			press(tea.KeyDown)
		}
		if s, ok := st.model.SelectedIssue(); ok && s.ID == id {
			return nil
		}
		return fmt.Errorf("issue %s not selectable in the TUI list", id)
	}

	// Given the TUI is open
	sc.Step(`^the TUI is open$`, func() error {
		st.model = tui.New(context.Background(), client.New(w.cfg))
		drain(st.model.Init())                           // initial issue-list load
		send(tea.WindowSizeMsg{Width: 100, Height: 100}) // size the viewport
		return st.model.LastError()
	})

	// When I create an issue
	sc.Step(`^I create an issue$`, func() error {
		ensureList()
		cmdKey('n') // open the create form
		typeStr("TUI created issue")
		press(tea.KeyTab)
		typeStr("created from the TUI")
		press(tea.KeyEnter) // submit -> create -> reload list
		return st.model.LastError()
	})

	// Then it is added with the same rules as the create story and appears in the list
	sc.Step(`^it is added with the same rules as the create story and appears in the list$`, func() error {
		lc, ok := st.model.LastCreated()
		if !ok {
			return fmt.Errorf("no issue was created through the TUI")
		}
		if lc.ID == "" {
			return fmt.Errorf("created issue has no id")
		}
		if lc.Type != domain.TypeTask || lc.Status != domain.StatusOpen || lc.Priority != domain.P2 {
			return fmt.Errorf("unexpected defaults: type=%q status=%q priority=%q", lc.Type, lc.Status, lc.Priority)
		}
		if lc.CreatedAt.IsZero() || lc.UpdatedAt.IsZero() {
			return fmt.Errorf("expected timestamps to be set")
		}
		if !containsIssue(st.model.Issues(), lc.ID) {
			return fmt.Errorf("created issue %s does not appear in the TUI list", lc.ID)
		}
		stored, err := w.verify.GetIssue(lc.ID)
		if err != nil {
			return fmt.Errorf("created issue not persisted: %w", err)
		}
		if stored.Subject != "TUI created issue" {
			return fmt.Errorf("persisted subject = %q, want %q", stored.Subject, "TUI created issue")
		}
		return nil
	})

	// When I edit its fields or change its status to closed
	// (the issue comes from the shared "an open issue" step)
	sc.Step(`^I edit its fields or change its status to closed$`, func() error {
		if err := selectIssue(w.issue.ID); err != nil {
			return err
		}
		cmdKey('e')         // open the edit form (fields: type, status, priority, owner)
		press(tea.KeyTab)   // move from type to status
		typeStr("closed")   // set only status; empty fields stay unchanged
		press(tea.KeyEnter) // submit -> update -> reload list
		return st.model.LastError()
	})

	// Then the change persists and the ledger records it
	sc.Step(`^the change persists and the ledger records it$`, func() error {
		stored, err := w.verify.GetIssue(w.issue.ID)
		if err != nil {
			return err
		}
		if stored.Status != domain.StatusClosed {
			return fmt.Errorf("status = %q, want closed", stored.Status)
		}
		history, err := w.verify.IssueHistory(w.issue.ID)
		if err != nil {
			return err
		}
		for _, e := range history {
			if e.Kind == domain.LedgerStatusChanged && e.NewValue == string(domain.StatusClosed) {
				return nil
			}
		}
		return fmt.Errorf("no status_changed->closed ledger entry found (%d entries)", len(history))
	})

	// When I create a child from it (the parent comes from the shared "an issue" step)
	sc.Step(`^I create a child from it$`, func() error {
		if err := selectIssue(w.issue.ID); err != nil {
			return err
		}
		cmdKey('c') // open the child form
		typeStr("Child of " + w.issue.ID)
		press(tea.KeyTab)
		typeStr("work carved out of the parent")
		press(tea.KeyEnter) // submit -> create child -> reload list
		lc, ok := st.model.LastCreated()
		if !ok {
			return fmt.Errorf("no child issue was created through the TUI")
		}
		st.child, st.hasChild = lc, true
		return st.model.LastError()
	})

	// Then the new issue's parent_id is set to it
	sc.Step(`^the new issue's parent_id is set to it$`, func() error {
		if !st.hasChild {
			return fmt.Errorf("no child issue recorded")
		}
		if st.child.ID == w.issue.ID {
			return fmt.Errorf("child id equals parent id")
		}
		if !containsIssue(st.model.Issues(), st.child.ID) {
			return fmt.Errorf("child issue %s does not appear in the TUI list", st.child.ID)
		}
		if _, err := w.verify.GetIssue(st.child.ID); err != nil {
			return fmt.Errorf("child issue not persisted: %w", err)
		}
		// The TUI create-child flow sets parent_id to the parent and sends it
		// through the client. NOTE: persisting parent_id server-side is Slice 4
		// (parent/child linking); this asserts the TUI wired the relationship.
		if st.child.ParentID == nil || *st.child.ParentID != w.issue.ID {
			return fmt.Errorf("child parent_id = %v, want %q", st.child.ParentID, w.issue.ID)
		}
		return nil
	})

	// When I view it (the issue comes from the shared "an issue" step). Give it a
	// document and a linked transcript first, then open its detail in the TUI and
	// add a comment, so the detail shows documents, comments, and transcripts.
	sc.Step(`^I view it$`, func() error {
		c := client.New(w.cfg)
		ctx := context.Background()
		if _, err := c.AttachDocument(ctx, w.issue.ID, string(domain.DocDesign), "Design notes", "# Design\n\nthe approach"); err != nil {
			return fmt.Errorf("attach document: %w", err)
		}
		if err := w.ingestFixture(); err != nil {
			return fmt.Errorf("ingest transcript: %w", err)
		}
		if _, err := c.LinkTranscript(ctx, w.transcript.ID, w.issue.ID); err != nil {
			return fmt.Errorf("link transcript: %w", err)
		}
		if err := selectIssue(w.issue.ID); err != nil {
			return err
		}
		press(tea.KeyEnter) // open detail -> load issue, documents, transcripts
		cmdKey('a')         // open the comment form
		typeStr("looks good to me")
		press(tea.KeyTab)
		typeStr("reviewer")
		press(tea.KeyEnter) // submit -> add comment -> shown in detail
		return st.model.LastError()
	})

	// Then I can see its documents, comments, and linked transcripts
	sc.Step(`^I can see its documents, comments, and linked transcripts$`, func() error {
		if _, ok := st.model.DetailIssue(); !ok {
			return fmt.Errorf("TUI is not showing an issue detail")
		}
		if len(st.model.DetailDocuments()) == 0 {
			return fmt.Errorf("detail shows no documents")
		}
		if len(st.model.DetailComments()) == 0 {
			return fmt.Errorf("detail shows no comments")
		}
		if len(st.model.DetailTranscripts()) == 0 {
			return fmt.Errorf("detail shows no linked transcripts")
		}
		view := st.model.View()
		for _, want := range []string{"Design notes", "looks good to me", "Fix the login bug"} {
			if !strings.Contains(view, want) {
				return fmt.Errorf("detail view missing %q:\n%s", want, view)
			}
		}
		return nil
	})
}
