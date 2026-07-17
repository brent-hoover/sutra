package main_test

// Step definitions for implemented scenarios (tagged @slice1). Each scenario
// drives the real binary surface: config.Load reads the environment, the
// actual Cobra `serve` command starts the daemon (graceful shutdown on ctx
// cancel), and `create`/`view` run as real Cobra commands whose --json output
// is parsed. A second service handle on the same DB verifies ledger writes.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/cli"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

type world struct {
	dir       string
	cfg       config.Config
	verify    *service.Service // second handle for reading ledger state
	serveCtx  context.Context
	cancel    context.CancelFunc
	serveCh   chan error // receives the serve command's exit error
	subject   string
	body      string
	issue     domain.Issue
	updated   domain.Issue   // issue as returned after an update
	issues    []domain.Issue // issues created for list/filter scenarios
	listOut   []domain.Issue // most recent list/filter result
	history   []domain.LedgerEntry
	viewOut   string
	err       error
	envBackup map[string]*string // original env values to restore in teardown

	// Document scenario state (slice 5).
	doc        domain.Document
	docContent string
	docOut     string
	attached   []domain.Document // documents attached in the current scenario
}

var managedEnvKeys = []string{"SUTRA_LISTEN", "SUTRA_DB", "SUTRA_HOST", "SUTRA_TOKEN"}

func freeLoopbackAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}

func (w *world) setup() error {
	*w = world{} // fresh per-scenario state

	dir, err := os.MkdirTemp("", "sutra-bdd-")
	if err != nil {
		return err
	}
	w.dir = dir

	// Snapshot then override the environment so config.Load is exercised and
	// prior values are restored in teardown (no cross-test order dependence).
	w.envBackup = make(map[string]*string, len(managedEnvKeys))
	for _, k := range managedEnvKeys {
		if v, ok := os.LookupEnv(k); ok {
			vv := v
			w.envBackup[k] = &vv
		} else {
			w.envBackup[k] = nil
		}
	}
	os.Setenv("SUTRA_DB", filepath.Join(dir, "test.db"))
	os.Unsetenv("SUTRA_TOKEN")

	// Start the daemon via the real Cobra `serve` command, retrying to tolerate
	// the rare race where the chosen ephemeral port is claimed by another
	// process between selection and the daemon's bind.
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		addr, err := freeLoopbackAddr()
		if err != nil {
			return err
		}
		os.Setenv("SUTRA_LISTEN", addr)
		os.Setenv("SUTRA_HOST", "http://"+addr)
		w.cfg = config.Load()

		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan error, 1)
		go func(cfg config.Config) {
			root := cli.NewRoot(cfg)
			root.SetArgs([]string{"serve"})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			ch <- root.ExecuteContext(ctx)
		}(w.cfg)

		exited, werr := waitListening(addr, ch)
		if werr == nil {
			w.serveCtx, w.cancel, w.serveCh = ctx, cancel, ch
			break
		}
		// Failed attempt: stop this daemon and drain its exit before retrying.
		cancel()
		if !exited {
			select {
			case <-ch:
			case <-time.After(2 * time.Second):
			}
		}
		lastErr = werr
	}
	if w.cancel == nil {
		return fmt.Errorf("could not start daemon: %w", lastErr)
	}

	var err2 error
	if w.verify, err2 = service.New(config.Config{DBPath: w.cfg.DBPath}); err2 != nil {
		return err2
	}
	return nil
}

// waitListening blocks until the daemon at addr accepts connections. It returns
// exited=true if the serve command exited (via ch) before it began listening.
func waitListening(addr string, ch chan error) (exited bool, err error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case e := <-ch:
			return true, fmt.Errorf("serve command exited before listening: %w", e)
		default:
		}
		if conn, e := net.DialTimeout("tcp", addr, 50*time.Millisecond); e == nil {
			conn.Close()
			return false, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false, fmt.Errorf("daemon did not start listening on %s", addr)
}

func (w *world) teardown() error {
	for k, v := range w.envBackup {
		if v == nil {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, *v)
		}
	}

	var errs []error
	shutdownConfirmed := w.cancel == nil
	if w.cancel != nil {
		w.cancel()
		select {
		case serveErr := <-w.serveCh:
			if serveErr != nil {
				errs = append(errs, fmt.Errorf("serve exited with error: %w", serveErr))
			}
			shutdownConfirmed = true
		case <-time.After(5 * time.Second):
			errs = append(errs, fmt.Errorf("timed out waiting for daemon to shut down"))
		}
	}
	if w.verify != nil {
		if err := w.verify.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close verify handle: %w", err))
		}
	}
	// Only remove the DB directory once the daemon has confirmed it stopped.
	if w.dir != "" {
		if shutdownConfirmed {
			if err := os.RemoveAll(w.dir); err != nil {
				errs = append(errs, fmt.Errorf("remove temp dir: %w", err))
			}
		} else {
			errs = append(errs, fmt.Errorf("left %s in place: shutdown unconfirmed", w.dir))
		}
	}
	return errors.Join(errs...)
}

// runCLI runs the real root command with args and returns its stdout.
func (w *world) runCLI(args ...string) (string, error) {
	root := cli.NewRoot(w.cfg)
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func (w *world) createIssue(subject, body string) {
	var out string
	out, w.err = w.runCLI("create", "--subject", subject, "--body", body, "--json")
	if w.err == nil {
		w.err = json.Unmarshal([]byte(strings.TrimSpace(out)), &w.issue)
	}
}

// attachDoc attaches a document via the real CLI, parsing the --json response
// into w.doc and recording it in w.attached.
func (w *world) attachDoc(issueID, kind, title, content string) {
	var out string
	out, w.err = w.runCLI("doc", "attach", issueID, "--kind", kind, "--title", title, "--content", content, "--json")
	if w.err == nil {
		w.err = json.Unmarshal([]byte(strings.TrimSpace(out)), &w.doc)
		if w.err == nil {
			w.attached = append(w.attached, w.doc)
		}
	}
}

// create runs the create command and returns the parsed issue (for scenarios
// that build several issues).
func (w *world) create(subject, body string) (domain.Issue, error) {
	out, err := w.runCLI("create", "--subject", subject, "--body", body, "--json")
	if err != nil {
		return domain.Issue{}, err
	}
	var i domain.Issue
	return i, json.Unmarshal([]byte(strings.TrimSpace(out)), &i)
}

// updateIssue runs `update <id>` with flags and returns the parsed issue.
func (w *world) updateIssue(id string, flags ...string) (domain.Issue, error) {
	args := append([]string{"update", id}, flags...)
	args = append(args, "--json")
	out, err := w.runCLI(args...)
	if err != nil {
		return domain.Issue{}, err
	}
	var i domain.Issue
	return i, json.Unmarshal([]byte(strings.TrimSpace(out)), &i)
}

// listIssues runs `list` with optional filter flags and returns the parsed set.
func (w *world) listIssues(flags ...string) ([]domain.Issue, error) {
	args := append([]string{"list"}, flags...)
	args = append(args, "--json")
	out, err := w.runCLI(args...)
	if err != nil {
		return nil, err
	}
	var issues []domain.Issue
	return issues, json.Unmarshal([]byte(strings.TrimSpace(out)), &issues)
}

func containsIssue(issues []domain.Issue, id string) bool {
	for _, i := range issues {
		if i.ID == id {
			return true
		}
	}
	return false
}

// InitializeScenario wires a fresh world per scenario and registers steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	w := &world{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, w.setup()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		return ctx, w.teardown()
	})

	// Create an issue
	sc.Step(`^no other input$`, func() error { return nil })
	sc.Step(`^I create an issue with a subject and body$`, func() error {
		w.createIssue("Skeleton subject", "Skeleton body")
		return nil
	})
	sc.Step(`^it is stored with a generated id and defaults type=task, status=open, priority=p2, and timestamps set$`, func() error {
		if w.err != nil {
			return fmt.Errorf("create failed: %w", w.err)
		}
		if w.issue.ID == "" {
			return fmt.Errorf("expected a generated id")
		}
		if w.issue.Type != domain.TypeTask || w.issue.Status != domain.StatusOpen || w.issue.Priority != domain.P2 {
			return fmt.Errorf("unexpected defaults: type=%q status=%q priority=%q", w.issue.Type, w.issue.Status, w.issue.Priority)
		}
		if w.issue.CreatedAt.IsZero() || w.issue.UpdatedAt.IsZero() {
			return fmt.Errorf("expected timestamps to be set")
		}
		return nil
	})
	sc.Step(`^a new issue is created$`, func() error {
		if w.issue.ID == "" {
			w.createIssue("Skeleton subject", "Skeleton body")
		}
		return w.err
	})
	sc.Step(`^it is stored$`, func() error { return nil })
	sc.Step(`^a LedgerEntry of kind created is appended$`, func() error {
		history, err := w.verify.IssueHistory(w.issue.ID)
		if err != nil {
			return err
		}
		for _, e := range history {
			if e.Kind == domain.LedgerCreated {
				return nil
			}
		}
		return fmt.Errorf("no LedgerEntry of kind created found (%d entries)", len(history))
	})
	sc.Step(`^a missing body$`, func() error {
		w.subject, w.body = "Has a subject", ""
		return nil
	})
	sc.Step(`^a missing subject$`, func() error {
		w.subject, w.body = "", "Has a body"
		return nil
	})
	sc.Step(`^I try to create the issue$`, func() error {
		w.createIssue(w.subject, w.body)
		return nil
	})
	sc.Step(`^it is rejected$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected creation to be rejected, got none")
		}
		return nil
	})

	// View an issue's core fields
	sc.Step(`^a stored issue exists$`, func() error {
		w.createIssue("View me", "the body to read back")
		return w.err
	})
	sc.Step(`^I view it by id$`, func() error {
		// Use the documented default invocation (no --json) so a human
		// renderer that drops fields cannot hide behind raw JSON.
		w.viewOut, w.err = w.runCLI("view", w.issue.ID)
		return nil
	})
	sc.Step(`^its core fields are returned$`, func() error {
		if w.err != nil {
			return fmt.Errorf("view failed: %w", w.err)
		}
		for label, want := range map[string]string{
			"id":       w.issue.ID,
			"subject":  w.issue.Subject,
			"body":     w.issue.Body,
			"type":     string(w.issue.Type),
			"status":   string(w.issue.Status),
			"priority": string(w.issue.Priority),
		} {
			if !strings.Contains(w.viewOut, want) {
				return fmt.Errorf("view output missing %s (%q):\n%s", label, want, w.viewOut)
			}
		}
		return nil
	})

	// Run the daemon
	sc.Step(`^a config with a listen address and DB path$`, func() error {
		if w.cfg.ListenAddr == "" || w.cfg.DBPath == "" {
			return fmt.Errorf("config not loaded")
		}
		return nil
	})
	sc.Step(`^I run sutra serve$`, func() error { return nil }) // started in setup via the real command
	sc.Step(`^it opens the store and serves HTTP on that address$`, func() error {
		w.createIssue("Ping", "Prove the daemon serves and the store is open")
		if w.err != nil {
			return fmt.Errorf("daemon did not serve a working request: %w", w.err)
		}
		if _, err := w.runCLI("view", w.issue.ID, "--json"); err != nil {
			return fmt.Errorf("daemon did not read back through the store: %w", err)
		}
		return nil
	})

	// --- Documents (slice 5) ---

	// Attach a document to an issue
	sc.Step(`^any issue$`, func() error {
		w.createIssue("Doc host", "an issue to hang documents from")
		return w.err
	})
	sc.Step(`^I attach a Document with a kind of problem, design, plan, or scenarios and markdown content$`, func() error {
		w.attachDoc(w.issue.ID, string(domain.DocProblem), "Problem statement", "# Problem\n\nSomething needs fixing.")
		return nil
	})
	sc.Step(`^it is stored linked to that issue with timestamps set$`, func() error {
		if w.err != nil {
			return fmt.Errorf("attach failed: %w", w.err)
		}
		if w.doc.ID == "" {
			return fmt.Errorf("expected a generated document id")
		}
		if w.doc.IssueID != w.issue.ID {
			return fmt.Errorf("document linked to %q, want issue %q", w.doc.IssueID, w.issue.ID)
		}
		if w.doc.CreatedAt.IsZero() || w.doc.UpdatedAt.IsZero() {
			return fmt.Errorf("expected timestamps to be set")
		}
		stored, err := w.verify.GetDocument(w.doc.ID)
		if err != nil {
			return fmt.Errorf("document not persisted: %w", err)
		}
		if stored.Content == "" {
			return fmt.Errorf("stored document has empty content")
		}
		return nil
	})
	sc.Step(`^empty content$`, func() error {
		w.docContent = ""
		return nil
	})
	sc.Step(`^I try to attach$`, func() error {
		w.attachDoc(w.issue.ID, string(domain.DocProblem), "Empty", w.docContent)
		return nil
	})

	// Read a document
	sc.Step(`^an issue with a document$`, func() error {
		w.createIssue("Doc host", "an issue with one document")
		if w.err != nil {
			return w.err
		}
		w.attachDoc(w.issue.ID, string(domain.DocDesign), "Design doc", "# Design\n\nThe approach.")
		return w.err
	})
	sc.Step(`^I open the document by id$`, func() error {
		// Default render (no --json) so a renderer that drops fields is caught.
		w.docOut, w.err = w.runCLI("doc", "read", w.doc.ID)
		return nil
	})
	sc.Step(`^its kind, title, and content are returned$`, func() error {
		if w.err != nil {
			return fmt.Errorf("read failed: %w", w.err)
		}
		for label, want := range map[string]string{
			"kind":    string(w.doc.Kind),
			"title":   w.doc.Title,
			"content": w.doc.Content,
		} {
			if !strings.Contains(w.docOut, want) {
				return fmt.Errorf("read output missing %s (%q):\n%s", label, want, w.docOut)
			}
		}
		return nil
	})

	// List an issue's documents
	sc.Step(`^an issue with several documents$`, func() error {
		w.createIssue("Doc host", "an issue with several documents")
		if w.err != nil {
			return w.err
		}
		w.attachDoc(w.issue.ID, string(domain.DocProblem), "The problem", "problem body")
		if w.err != nil {
			return w.err
		}
		w.attachDoc(w.issue.ID, string(domain.DocDesign), "The design", "design body")
		if w.err != nil {
			return w.err
		}
		w.attachDoc(w.issue.ID, string(domain.DocPlan), "The plan", "plan body")
		return w.err
	})
	sc.Step(`^I list its documents$`, func() error {
		w.docOut, w.err = w.runCLI("doc", "list", w.issue.ID)
		return nil
	})
	sc.Step(`^all appear with their kind and title$`, func() error {
		if w.err != nil {
			return fmt.Errorf("list failed: %w", w.err)
		}
		for _, d := range w.attached {
			if !strings.Contains(w.docOut, string(d.Kind)) {
				return fmt.Errorf("list output missing kind %q:\n%s", d.Kind, w.docOut)
			}
			if !strings.Contains(w.docOut, d.Title) {
				return fmt.Errorf("list output missing title %q:\n%s", d.Title, w.docOut)
			}
		}
		return nil
	})

	// Update a document
	sc.Step(`^an existing document$`, func() error {
		w.createIssue("Doc host", "an issue with a document to update")
		if w.err != nil {
			return w.err
		}
		w.attachDoc(w.issue.ID, string(domain.DocPlan), "Plan doc", "original content")
		return w.err
	})
	sc.Step(`^I update its content$`, func() error {
		_, w.err = w.runCLI("doc", "update", w.doc.ID, "--content", "revised content", "--json")
		return nil
	})
	sc.Step(`^the new content is stored and updated_at advances$`, func() error {
		if w.err != nil {
			return fmt.Errorf("update failed: %w", w.err)
		}
		stored, err := w.verify.GetDocument(w.doc.ID)
		if err != nil {
			return err
		}
		if stored.Content != "revised content" {
			return fmt.Errorf("content = %q, want %q", stored.Content, "revised content")
		}
		if !stored.UpdatedAt.After(w.doc.UpdatedAt) {
			return fmt.Errorf("updated_at did not advance: was %s, now %s", w.doc.UpdatedAt, stored.UpdatedAt)
		}
		return nil
	})

	// Remove a document
	sc.Step(`^I remove it$`, func() error {
		_, w.err = w.runCLI("doc", "remove", w.doc.ID)
		return w.err
	})
	sc.Step(`^it no longer appears in the issue's document list$`, func() error {
		docs, err := w.verify.ListDocuments(w.issue.ID)
		if err != nil {
			return err
		}
		for _, d := range docs {
			if d.ID == w.doc.ID {
				return fmt.Errorf("removed document %s still appears in the list", w.doc.ID)
			}
		}
		return nil
	})

	registerSlice3Steps(sc, w)
}

// registerSlice3Steps wires the step definitions for the @slice3 scenarios:
// list, update, comment, soft-delete, history, and filter.
func registerSlice3Steps(sc *godog.ScenarioContext, w *world) {
	// --- List issues ---
	sc.Step(`^several live issues exist$`, func() error {
		w.issues = nil
		for _, s := range []string{"first", "second", "third"} {
			i, err := w.create(s+" subject", s+" body")
			if err != nil {
				return err
			}
			w.issues = append(w.issues, i)
		}
		return nil
	})
	sc.Step(`^I list issues$`, func() error {
		w.listOut, w.err = w.listIssues()
		return w.err
	})
	sc.Step(`^all live issues are returned$`, func() error {
		if len(w.listOut) != len(w.issues) {
			return fmt.Errorf("expected %d issues, got %d", len(w.issues), len(w.listOut))
		}
		for _, want := range w.issues {
			if !containsIssue(w.listOut, want.ID) {
				return fmt.Errorf("issue %s missing from list", want.ID)
			}
		}
		return nil
	})
	sc.Step(`^one of those issues is soft-deleted$`, func() error {
		if _, err := w.runCLI("delete", w.issues[0].ID, "--json"); err != nil {
			return err
		}
		return nil
	})
	sc.Step(`^the soft-deleted one is excluded by default$`, func() error {
		if containsIssue(w.listOut, w.issues[0].ID) {
			return fmt.Errorf("soft-deleted issue %s still listed", w.issues[0].ID)
		}
		for _, want := range w.issues[1:] {
			if !containsIssue(w.listOut, want.ID) {
				return fmt.Errorf("live issue %s missing from list", want.ID)
			}
		}
		return nil
	})

	// --- Update issue fields ---
	sc.Step(`^an open issue$`, func() error {
		w.issue, w.err = w.create("Update me", "body to update")
		return w.err
	})
	sc.Step(`^I change its status to in_progress$`, func() error {
		w.updated, w.err = w.updateIssue(w.issue.ID, "--status", "in_progress")
		return w.err
	})
	sc.Step(`^the field updates and updated_at advances$`, func() error {
		if w.updated.Status != domain.StatusInProgress {
			return fmt.Errorf("status = %q, want in_progress", w.updated.Status)
		}
		if !w.updated.UpdatedAt.After(w.issue.UpdatedAt) {
			return fmt.Errorf("updated_at did not advance: before=%s after=%s", w.issue.UpdatedAt, w.updated.UpdatedAt)
		}
		return nil
	})
	sc.Step(`^a field on an issue changes$`, func() error {
		// Reuse the change already made above if present; otherwise make one.
		if w.updated.ID == "" {
			w.issue, w.err = w.create("Change me", "body")
			if w.err != nil {
				return w.err
			}
			w.updated, w.err = w.updateIssue(w.issue.ID, "--status", "in_progress")
		}
		return w.err
	})
	sc.Step(`^the change is saved$`, func() error { return nil }) // persisted by the preceding step
	sc.Step(`^a LedgerEntry of kind status_changed with field, old_value, and new_value is appended$`, func() error {
		history, err := w.verify.IssueHistory(w.issue.ID)
		if err != nil {
			return err
		}
		for _, e := range history {
			if e.Kind != domain.LedgerStatusChanged {
				continue
			}
			if e.Field != "status" {
				return fmt.Errorf("status_changed field = %q, want status", e.Field)
			}
			if e.OldValue != "open" || e.NewValue != "in_progress" {
				return fmt.Errorf("status_changed old/new = %q/%q, want open/in_progress", e.OldValue, e.NewValue)
			}
			return nil
		}
		return fmt.Errorf("no status_changed LedgerEntry found (%d entries)", len(history))
	})

	// --- Comment / Soft-delete share the "an issue" step ---
	sc.Step(`^an issue$`, func() error {
		w.issue, w.err = w.create("Discuss me", "body for comments and deletes")
		return w.err
	})

	// --- Comment on an issue ---
	sc.Step(`^I add a comment with a body$`, func() error {
		_, w.err = w.runCLI("comment", w.issue.ID, "--body", "a considered comment", "--json")
		return w.err
	})
	sc.Step(`^a Comment is stored and a LedgerEntry of kind commented is appended$`, func() error {
		comments, err := w.verify.IssueComments(w.issue.ID)
		if err != nil {
			return err
		}
		if len(comments) == 0 {
			return fmt.Errorf("no comment stored")
		}
		if comments[0].Body != "a considered comment" {
			return fmt.Errorf("comment body = %q", comments[0].Body)
		}
		history, err := w.verify.IssueHistory(w.issue.ID)
		if err != nil {
			return err
		}
		for _, e := range history {
			if e.Kind == domain.LedgerCommented {
				return nil
			}
		}
		return fmt.Errorf("no commented LedgerEntry found (%d entries)", len(history))
	})

	// --- Soft-delete an issue ---
	sc.Step(`^I delete it$`, func() error {
		_, w.err = w.runCLI("delete", w.issue.ID, "--json")
		return w.err
	})
	sc.Step(`^deleted_at is set and it drops from default lists and search$`, func() error {
		stored, err := w.verify.GetIssue(w.issue.ID)
		if err != nil {
			return err
		}
		if stored.DeletedAt == nil {
			return fmt.Errorf("deleted_at not set")
		}
		listed, err := w.listIssues()
		if err != nil {
			return err
		}
		if containsIssue(listed, w.issue.ID) {
			return fmt.Errorf("deleted issue %s still appears in default list", w.issue.ID)
		}
		return nil
	})
	sc.Step(`^an issue is deleted$`, func() error {
		if w.issue.ID == "" {
			w.issue, w.err = w.create("Delete me", "body")
			if w.err != nil {
				return w.err
			}
		}
		if _, err := w.runCLI("delete", w.issue.ID, "--json"); err != nil {
			return err
		}
		return nil
	})
	sc.Step(`^a LedgerEntry of kind deleted is appended$`, func() error {
		history, err := w.verify.IssueHistory(w.issue.ID)
		if err != nil {
			return err
		}
		for _, e := range history {
			if e.Kind == domain.LedgerDeleted {
				return nil
			}
		}
		return fmt.Errorf("no deleted LedgerEntry found (%d entries)", len(history))
	})

	// --- View an issue's change history ---
	sc.Step(`^an issue with several changes$`, func() error {
		w.issue, w.err = w.create("History me", "body")
		if w.err != nil {
			return w.err
		}
		for _, flags := range [][]string{
			{"--status", "in_progress"},
			{"--priority", "p1"},
			{"--owner", "agent-x"},
		} {
			if _, err := w.updateIssue(w.issue.ID, flags...); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^I view its history$`, func() error {
		out, err := w.runCLI("history", w.issue.ID, "--json")
		if err != nil {
			w.err = err
			return err
		}
		w.err = json.Unmarshal([]byte(strings.TrimSpace(out)), &w.history)
		return w.err
	})
	sc.Step(`^LedgerEntry rows appear in chronological order$`, func() error {
		if len(w.history) < 2 {
			return fmt.Errorf("expected several ledger rows, got %d", len(w.history))
		}
		for i := 1; i < len(w.history); i++ {
			if w.history[i].At.Before(w.history[i-1].At) {
				return fmt.Errorf("ledger not chronological at index %d", i)
			}
		}
		return nil
	})

	// --- Filter issues ---
	sc.Step(`^issues with varied status, type, priority, labels, and owner$`, func() error {
		w.issues = nil
		// I1: bug + open (default status)
		i1, err := w.create("bug open", "body")
		if err != nil {
			return err
		}
		if i1, err = w.updateIssue(i1.ID, "--type", "bug"); err != nil {
			return err
		}
		// I2: bug + in_progress
		i2, err := w.create("bug in progress", "body")
		if err != nil {
			return err
		}
		if i2, err = w.updateIssue(i2.ID, "--type", "bug", "--status", "in_progress"); err != nil {
			return err
		}
		// I3: task + open (defaults)
		i3, err := w.create("task open", "body")
		if err != nil {
			return err
		}
		w.issues = []domain.Issue{i1, i2, i3}
		return nil
	})
	sc.Step(`^I list with a filter such as status=open, label=bug, or owner=AGENT$`, func() error {
		// Label filtering arrives with labels in slice 4; filter on an
		// S3-supported field here.
		w.listOut, w.err = w.listIssues("--status", "open")
		return w.err
	})
	sc.Step(`^only matching, non-soft-deleted issues are returned$`, func() error {
		if len(w.listOut) == 0 {
			return fmt.Errorf("expected matches, got none")
		}
		for _, i := range w.listOut {
			if i.Status != domain.StatusOpen {
				return fmt.Errorf("issue %s has status %q, want open", i.ID, i.Status)
			}
			if i.DeletedAt != nil {
				return fmt.Errorf("soft-deleted issue %s returned", i.ID)
			}
		}
		return nil
	})
	sc.Step(`^multiple filters$`, func() error { return nil })
	sc.Step(`^I combine them$`, func() error {
		w.listOut, w.err = w.listIssues("--type", "bug", "--status", "open")
		return w.err
	})
	sc.Step(`^results match all filters using AND$`, func() error {
		if len(w.listOut) == 0 {
			return fmt.Errorf("expected AND matches, got none")
		}
		for _, i := range w.listOut {
			if i.Type != domain.TypeBug || i.Status != domain.StatusOpen {
				return fmt.Errorf("issue %s (type=%q status=%q) violates AND filter", i.ID, i.Type, i.Status)
			}
		}
		// I2 (bug/in_progress) and I3 (task/open) must be excluded.
		if containsIssue(w.listOut, w.issues[1].ID) || containsIssue(w.listOut, w.issues[2].ID) {
			return fmt.Errorf("AND filter returned non-matching issues")
		}
		return nil
	})
}
