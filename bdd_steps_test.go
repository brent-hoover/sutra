package main_test

// Step definitions for the implemented scenarios, tagged @slice1 (create,
// view, serve), @slice3 (list, update, comment, soft-delete, history, filter),
// @slice5 (document attach/read/list/update/remove), and @slice6 (transcript
// ingest/re-ingest/link/view/read/discover). Each scenario drives the real
// binary surface: config.Load reads the environment, the actual Cobra `serve`
// command starts the daemon (graceful shutdown on ctx cancel), and the CLI
// subcommands run as real Cobra commands whose --json output is parsed. A
// second service handle on the same DB verifies ledger and stored state.

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

	// transcript-capture (slice 6) state
	sessionID   string
	sessionPath string
	transcript  domain.Transcript
	prevID      string
	prevCount   int
	projectDir  string
	sessionA    string
	sessionB    string
	txOut       string

	// Document scenario state (slice 5).
	doc        domain.Document
	docContent string
	docOut     string
	attached   []domain.Document // documents attached in the current scenario

	// Search scenario state (slice 7).
	searchTerm    string
	searchResults domain.SearchResults
	srchIssueID   string
	srchDocID     string
	srchLiveIssue string
	srchDelIssue  string
	srchMsgSeq    int

	// Linking & labels state (slice 4).
	linkA       domain.Issue
	linkB       domain.Issue
	linkC       domain.Issue
	viewLabel   string
	viewRelated string
	viewBlocker string
	viewComment string
}

var managedEnvKeys = []string{"SUTRA_LISTEN", "SUTRA_DB", "SUTRA_HOST", "SUTRA_TOKEN", "SUTRA_PROJECTS_DIR"}

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
	// Run the whole suite with bearer auth enabled: the daemon enforces it and
	// the client (config.Load) sends it, so every scenario exercises the
	// authenticated path (slice 2).
	os.Setenv("SUTRA_TOKEN", "test-secret-token")
	// Transcript ingest/discover is confined to the projects dir; point it at the
	// scenario temp dir where session fixtures are written.
	os.Setenv("SUTRA_PROJECTS_DIR", dir)

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
	if w.verify, err2 = service.New(config.Config{DBPath: w.cfg.DBPath, ProjectsDir: w.cfg.ProjectsDir}); err2 != nil {
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

// fixtureLines returns a small Claude-style JSONL transcript exercising a
// user message, an assistant text reply, and a timestamp-less tool event.
func fixtureLines() []string {
	return []string{
		`{"type":"user","message":{"role":"user","content":"Fix the login bug"},"timestamp":"2026-07-16T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I will fix it."}]},"timestamp":"2026-07-16T10:00:05Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{}}]}}`,
	}
}

// writeSession writes a session .jsonl file under dir and returns its path.
func writeSession(dir, sessionID string, lines []string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ingest ingests a session file through the real CLI and decodes the result.
func (w *world) ingest(path string) error {
	out, err := w.runCLI("transcript", "ingest", path, "--json")
	w.err = err
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(strings.TrimSpace(out)), &w.transcript)
}

// ingestFixture writes the standard fixture under ~/.claude/projects-style
// dirs inside the temp dir and ingests it.
func (w *world) ingestFixture() error {
	w.sessionID = "11111111-2222-3333-4444-555555555555"
	dir := filepath.Join(w.dir, "projects", "-Users-me-proj")
	path, err := writeSession(dir, w.sessionID, fixtureLines())
	if err != nil {
		return err
	}
	w.sessionPath = path
	return w.ingest(path)
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

// ingestLines writes a session file with the given lines under the projects
// dir and ingests it through the real CLI.
func (w *world) ingestLines(sessionID string, lines []string) error {
	dir := filepath.Join(w.dir, "projects", "-Users-me-"+sessionID)
	path, err := writeSession(dir, sessionID, lines)
	if err != nil {
		return err
	}
	return w.ingest(path)
}

// search runs the real `search` command with optional flags and decodes the
// --json result.
func (w *world) search(term string, flags ...string) (domain.SearchResults, error) {
	args := append([]string{"search", term}, flags...)
	args = append(args, "--json")
	out, err := w.runCLI(args...)
	if err != nil {
		return domain.SearchResults{}, err
	}
	var res domain.SearchResults
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &res)
	return res, err
}

// create runs the create command and returns the parsed issue (for scenarios
// that build several issues).
func (w *world) create(subject, body string) (domain.Issue, error) {
	out, err := w.runCLI("create", "--subject", subject, "--body", body, "--json")
	if err != nil {
		return domain.Issue{}, err
	}
	var i domain.Issue
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &i)
	return i, err
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
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &i)
	return i, err
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
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &issues)
	return issues, err
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

	// Ingest a transcript
	sc.Step(`^a session file under ~/\.claude/projects$`, func() error {
		w.sessionID = "11111111-2222-3333-4444-555555555555"
		dir := filepath.Join(w.dir, "projects", "-Users-me-proj")
		path, err := writeSession(dir, w.sessionID, fixtureLines())
		if err != nil {
			return err
		}
		w.sessionPath = path
		return nil
	})
	sc.Step(`^I ingest it$`, func() error { return w.ingest(w.sessionPath) })
	sc.Step(`^a Transcript is stored with session_id, source_path, captured_at, and a title derived from the first user message$`, func() error {
		if w.err != nil {
			return fmt.Errorf("ingest failed: %w", w.err)
		}
		if w.transcript.SessionID != w.sessionID {
			return fmt.Errorf("session_id = %q, want %q", w.transcript.SessionID, w.sessionID)
		}
		// Ingest stores the logical path under the projects dir (not symlink-
		// resolved); the pinned projects-dir handle is what enforces confinement.
		wantPath, _ := filepath.Abs(w.sessionPath)
		if w.transcript.SourcePath != wantPath {
			return fmt.Errorf("source_path = %q, want %q", w.transcript.SourcePath, wantPath)
		}
		if w.transcript.CapturedAt.IsZero() {
			return fmt.Errorf("captured_at not set")
		}
		if w.transcript.Title != "Fix the login bug" {
			return fmt.Errorf("title = %q, want derived from first user message", w.transcript.Title)
		}
		return nil
	})
	sc.Step(`^that file's lines$`, func() error { return nil })
	sc.Step(`^ingestion runs$`, func() error { return nil })
	sc.Step(`^each line becomes a Message with seq, role, extracted text, original raw, and at when present$`, func() error {
		msgs := w.transcript.Messages
		if len(msgs) != 3 {
			return fmt.Errorf("expected 3 messages, got %d", len(msgs))
		}
		wantRoles := []domain.Role{domain.RoleUser, domain.RoleAssistant, domain.RoleTool}
		for i, m := range msgs {
			if m.Seq != i {
				return fmt.Errorf("message %d has seq %d", i, m.Seq)
			}
			if m.Role != wantRoles[i] {
				return fmt.Errorf("message %d role = %q, want %q", i, m.Role, wantRoles[i])
			}
			if m.Raw == "" {
				return fmt.Errorf("message %d has empty raw", i)
			}
		}
		if msgs[0].Text != "Fix the login bug" || msgs[1].Text != "I will fix it." {
			return fmt.Errorf("extracted text mismatch: %q / %q", msgs[0].Text, msgs[1].Text)
		}
		if msgs[0].At == nil || msgs[1].At == nil {
			return fmt.Errorf("expected timestamps on the first two messages")
		}
		if msgs[2].At != nil {
			return fmt.Errorf("expected no timestamp on the timestamp-less tool event")
		}
		return nil
	})

	// Re-ingest is idempotent
	sc.Step(`^a transcript already ingested$`, func() error {
		if err := w.ingestFixture(); err != nil {
			return err
		}
		w.prevID = w.transcript.ID
		w.prevCount = len(w.transcript.Messages)
		return nil
	})
	sc.Step(`^I ingest the same session_id again$`, func() error { return w.ingest(w.sessionPath) })
	sc.Step(`^the existing Transcript and its Message rows are updated in place, not duplicated$`, func() error {
		if w.transcript.ID != w.prevID {
			return fmt.Errorf("re-ingest created a new transcript id %q (was %q)", w.transcript.ID, w.prevID)
		}
		if len(w.transcript.Messages) != w.prevCount {
			return fmt.Errorf("message count changed: %d, want %d", len(w.transcript.Messages), w.prevCount)
		}
		// Read it back through the daemon to confirm the DB has no duplicates.
		out, err := w.runCLI("transcript", "view", w.transcript.ID, "--json")
		if err != nil {
			return err
		}
		var stored domain.Transcript
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &stored); err != nil {
			return err
		}
		if len(stored.Messages) != w.prevCount {
			return fmt.Errorf("stored message count = %d, want %d (duplicated?)", len(stored.Messages), w.prevCount)
		}
		return nil
	})

	// Link a transcript to an issue
	sc.Step(`^an ingested transcript and an issue$`, func() error {
		if err := w.ingestFixture(); err != nil {
			return err
		}
		w.createIssue("Linkable", "issue to link a transcript to")
		return w.err
	})
	sc.Step(`^I link them$`, func() error {
		out, err := w.runCLI("transcript", "link", w.transcript.ID, "--issue", w.issue.ID, "--json")
		w.err = err
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(strings.TrimSpace(out)), &w.transcript)
	})
	sc.Step(`^the transcript's issue_id is set and a LedgerEntry of kind linked is appended to the issue$`, func() error {
		if w.transcript.IssueID == nil || *w.transcript.IssueID != w.issue.ID {
			return fmt.Errorf("transcript issue_id = %v, want %q", w.transcript.IssueID, w.issue.ID)
		}
		history, err := w.verify.IssueHistory(w.issue.ID)
		if err != nil {
			return err
		}
		for _, e := range history {
			if e.Kind == domain.LedgerLinked {
				return nil
			}
		}
		return fmt.Errorf("no LedgerEntry of kind linked found (%d entries)", len(history))
	})

	// View an issue's transcripts
	sc.Step(`^an issue with linked transcripts$`, func() error {
		if err := w.ingestFixture(); err != nil {
			return err
		}
		w.createIssue("Owns transcripts", "issue that owns a transcript")
		if w.err != nil {
			return w.err
		}
		_, err := w.runCLI("transcript", "link", w.transcript.ID, "--issue", w.issue.ID, "--json")
		return err
	})
	sc.Step(`^I view its transcripts$`, func() error {
		w.txOut, w.err = w.runCLI("transcript", "list", "--issue", w.issue.ID)
		return w.err
	})
	sc.Step(`^each linked Transcript appears with its title and captured_at$`, func() error {
		if !strings.Contains(w.txOut, "Fix the login bug") {
			return fmt.Errorf("list output missing title:\n%s", w.txOut)
		}
		if !strings.Contains(w.txOut, "2026-07-16") {
			return fmt.Errorf("list output missing captured_at:\n%s", w.txOut)
		}
		return nil
	})

	// Read a transcript
	sc.Step(`^an ingested transcript$`, func() error { return w.ingestFixture() })
	sc.Step(`^I open it$`, func() error {
		w.txOut, w.err = w.runCLI("transcript", "view", w.transcript.ID)
		return w.err
	})
	sc.Step(`^its Message rows render in seq order by role, reconstructed from raw$`, func() error {
		user := strings.Index(w.txOut, "Fix the login bug")
		asst := strings.Index(w.txOut, "I will fix it.")
		if user < 0 || asst < 0 || user > asst {
			return fmt.Errorf("messages not rendered in seq order:\n%s", w.txOut)
		}
		for _, role := range []string{"user", "assistant", "tool"} {
			if !strings.Contains(w.txOut, role) {
				return fmt.Errorf("output missing role %q:\n%s", role, w.txOut)
			}
		}
		// The timestamp-less tool event has no extracted text, so it must be
		// reconstructed from its raw JSON line.
		if !strings.Contains(w.txOut, "tool_use") {
			return fmt.Errorf("tool message not reconstructed from raw:\n%s", w.txOut)
		}
		return nil
	})

	// Discover local transcripts
	sc.Step(`^session files on disk$`, func() error {
		w.projectDir = filepath.Join(w.dir, "discover")
		w.sessionA = "aaaaaaaa-0000-0000-0000-000000000001"
		w.sessionB = "bbbbbbbb-0000-0000-0000-000000000002"
		pathA, err := writeSession(w.projectDir, w.sessionA, fixtureLines())
		if err != nil {
			return err
		}
		if _, err := writeSession(w.projectDir, w.sessionB, fixtureLines()); err != nil {
			return err
		}
		// Ingest only A, leaving B not-ingested.
		return w.ingest(pathA)
	})
	sc.Step(`^I list available transcripts, optionally by project dir$`, func() error {
		w.txOut, w.err = w.runCLI("transcript", "discover", "--dir", w.projectDir)
		return w.err
	})
	sc.Step(`^each file's session_id, path, and ingested state is shown$`, func() error {
		if !strings.Contains(w.txOut, w.sessionA) || !strings.Contains(w.txOut, w.sessionB) {
			return fmt.Errorf("discover output missing a session_id:\n%s", w.txOut)
		}
		if !strings.Contains(w.txOut, filepath.Join(w.projectDir, w.sessionA+".jsonl")) {
			return fmt.Errorf("discover output missing a path:\n%s", w.txOut)
		}
		if !strings.Contains(w.txOut, "not-ingested") || !strings.Contains(w.txOut, "\tingested\t") {
			return fmt.Errorf("discover output missing ingested state:\n%s", w.txOut)
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
		w.attachDoc(w.issue.ID, string(domain.DocProblem), "Root cause writeup", "problem body")
		if w.err != nil {
			return w.err
		}
		w.attachDoc(w.issue.ID, string(domain.DocDesign), "Chosen approach", "design body")
		if w.err != nil {
			return w.err
		}
		w.attachDoc(w.issue.ID, string(domain.DocPlan), "Rollout steps", "plan body")
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
	registerSlice8Steps(sc, w)
	registerSlice2Steps(sc, w)
	registerSlice7Steps(sc, w)
}

// registerSlice7Steps wires the step definitions for the @slice7 scenarios:
// cross-entity search, soft-delete exclusion, message context, ranking, and
// scoped search. Each seeds known terms across entities and drives the real
// `search` command, asserting on the parsed JSON results.
func registerSlice7Steps(sc *godog.ScenarioContext, w *world) {
	// Shared When across scenarios 1-4.
	sc.Step(`^I search for that term$`, func() error {
		w.searchResults, w.err = w.search(w.searchTerm)
		return w.err
	})

	// --- Full-text search across everything ---
	sc.Step(`^a known term appears in an issue, a document, and a transcript message$`, func() error {
		w.searchTerm = "flibbertigibbet"
		iss, err := w.create("cross entity issue", "body mentions "+w.searchTerm+" here")
		if err != nil {
			return err
		}
		w.srchIssueID = iss.ID
		// Attach the document to a separate host issue whose own text does NOT
		// contain the term, so the document hit is a distinct entity from the
		// issue hit.
		host, err := w.create("doc host", "an issue to hang a document from")
		if err != nil {
			return err
		}
		w.attachDoc(host.ID, string(domain.DocDesign), "Design", "design content with "+w.searchTerm+" inside")
		if w.err != nil {
			return w.err
		}
		w.srchDocID = w.doc.ID
		return w.ingestLines("cross000-0000-0000-0000-000000000001", []string{
			`{"type":"user","message":{"role":"user","content":"discussing ` + w.searchTerm + ` in chat"},"timestamp":"2026-07-16T10:00:00Z"}`,
		})
	})
	sc.Step(`^hits from the issue, the document, and the message all appear$`, func() error {
		if w.err != nil {
			return w.err
		}
		var foundIssue, foundDoc, foundMsg bool
		for _, h := range w.searchResults.Hits {
			switch {
			case h.Kind == domain.KindIssue && h.Issue != nil && h.Issue.ID == w.srchIssueID:
				foundIssue = true
			case h.Kind == domain.KindDocument && h.Document != nil && h.Document.ID == w.srchDocID:
				foundDoc = true
			case h.Kind == domain.KindMessage && h.Message != nil && strings.Contains(h.Message.Text, w.searchTerm):
				foundMsg = true
			}
		}
		if !foundIssue || !foundDoc || !foundMsg {
			return fmt.Errorf("missing kind: issue=%v doc=%v msg=%v (hits=%d)", foundIssue, foundDoc, foundMsg, len(w.searchResults.Hits))
		}
		return nil
	})

	// --- Soft-deleted content is excluded ---
	sc.Step(`^a known term appears in a live issue and in a soft-deleted issue$`, func() error {
		w.searchTerm = "vorpalsword"
		live, err := w.create("live issue", "body with "+w.searchTerm)
		if err != nil {
			return err
		}
		w.srchLiveIssue = live.ID
		del, err := w.create("deleted issue", "body with "+w.searchTerm)
		if err != nil {
			return err
		}
		w.srchDelIssue = del.ID
		_, err = w.runCLI("delete", del.ID, "--json")
		return err
	})
	sc.Step(`^the live issue appears and the soft-deleted issue is excluded$`, func() error {
		if w.err != nil {
			return w.err
		}
		var foundLive, foundDeleted bool
		for _, h := range w.searchResults.Hits {
			if h.Kind == domain.KindIssue && h.Issue != nil {
				if h.Issue.ID == w.srchLiveIssue {
					foundLive = true
				}
				if h.Issue.ID == w.srchDelIssue {
					foundDeleted = true
				}
			}
		}
		if !foundLive {
			return fmt.Errorf("live issue %s missing from results", w.srchLiveIssue)
		}
		if foundDeleted {
			return fmt.Errorf("soft-deleted issue %s must be excluded from search", w.srchDelIssue)
		}
		return nil
	})

	// --- Message hits carry context ---
	sc.Step(`^a transcript whose middle message contains a known term$`, func() error {
		w.searchTerm = "jabberwocky"
		w.srchMsgSeq = 2
		return w.ingestLines("ctxt0000-0000-0000-0000-000000000001", []string{
			`{"type":"user","message":{"role":"user","content":"first message"},"timestamp":"2026-07-16T10:00:00Z"}`,
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"second message"}]},"timestamp":"2026-07-16T10:00:01Z"}`,
			`{"type":"user","message":{"role":"user","content":"the ` + w.searchTerm + ` middle message"},"timestamp":"2026-07-16T10:00:02Z"}`,
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"fourth message"}]},"timestamp":"2026-07-16T10:00:03Z"}`,
			`{"type":"user","message":{"role":"user","content":"fifth message"},"timestamp":"2026-07-16T10:00:04Z"}`,
		})
	})
	sc.Step(`^the message hit includes its role, text, seq, owning transcript, and adjacent messages$`, func() error {
		if w.err != nil {
			return w.err
		}
		var hit *domain.SearchHit
		for i := range w.searchResults.Hits {
			h := &w.searchResults.Hits[i]
			if h.Kind == domain.KindMessage && h.Message != nil && strings.Contains(h.Message.Text, w.searchTerm) {
				hit = h
				break
			}
		}
		if hit == nil {
			return fmt.Errorf("no message hit found for term")
		}
		if hit.Message.Role == "" {
			return fmt.Errorf("message hit missing role")
		}
		if hit.Message.Seq != w.srchMsgSeq {
			return fmt.Errorf("message seq = %d, want %d", hit.Message.Seq, w.srchMsgSeq)
		}
		if hit.Transcript == nil {
			return fmt.Errorf("message hit missing owning transcript")
		}
		ctxSeqs := map[int]bool{}
		for _, c := range hit.Context {
			ctxSeqs[c.Seq] = true
		}
		if !ctxSeqs[1] || !ctxSeqs[3] {
			return fmt.Errorf("context missing adjacent messages, got seqs %v", ctxSeqs)
		}
		if ctxSeqs[w.srchMsgSeq] {
			return fmt.Errorf("context must not include the matching message itself (seq %d)", w.srchMsgSeq)
		}
		return nil
	})

	// --- Ranked results ---
	sc.Step(`^several matches of differing relevance for a term$`, func() error {
		w.searchTerm = "brillig"
		host, err := w.create("ranking host", "issue to host ranking documents")
		if err != nil {
			return err
		}
		w.attachDoc(host.ID, string(domain.DocProblem), "sparse", "mentions "+w.searchTerm+" once only")
		if w.err != nil {
			return w.err
		}
		dense := strings.Repeat(w.searchTerm+" ", 8)
		w.attachDoc(host.ID, string(domain.DocDesign), "dense", dense+"and more context")
		if w.err != nil {
			return w.err
		}
		w.srchDocID = w.doc.ID // the dense doc — expected to rank first
		return nil
	})
	sc.Step(`^results are ordered by relevance and each hit shows its source kind$`, func() error {
		if w.err != nil {
			return w.err
		}
		if len(w.searchResults.Hits) < 2 {
			return fmt.Errorf("expected multiple hits, got %d", len(w.searchResults.Hits))
		}
		for i := 1; i < len(w.searchResults.Hits); i++ {
			if w.searchResults.Hits[i].Rank < w.searchResults.Hits[i-1].Rank {
				return fmt.Errorf("hits not ordered by rank at index %d: %v < %v",
					i, w.searchResults.Hits[i].Rank, w.searchResults.Hits[i-1].Rank)
			}
		}
		for _, h := range w.searchResults.Hits {
			if !h.Kind.Valid() {
				return fmt.Errorf("hit has invalid source kind %q", h.Kind)
			}
		}
		top := w.searchResults.Hits[0]
		if top.Kind != domain.KindDocument || top.Document == nil || top.Document.ID != w.srchDocID {
			return fmt.Errorf("most relevant hit should be the dense document %s", w.srchDocID)
		}
		return nil
	})

	// --- Scoped search ---
	sc.Step(`^a known term appears in both an issue and a document$`, func() error {
		w.searchTerm = "slithytove"
		iss, err := w.create("scoped issue", "body with "+w.searchTerm)
		if err != nil {
			return err
		}
		w.srchIssueID = iss.ID
		w.attachDoc(iss.ID, string(domain.DocPlan), "scoped doc", "content with "+w.searchTerm)
		return w.err
	})
	sc.Step(`^I search for that term restricted to kind document$`, func() error {
		w.searchResults, w.err = w.search(w.searchTerm, "--kind", "document")
		return w.err
	})
	sc.Step(`^only document hits are returned$`, func() error {
		if w.err != nil {
			return w.err
		}
		if len(w.searchResults.Hits) == 0 {
			return fmt.Errorf("scoped search returned no hits")
		}
		for _, h := range w.searchResults.Hits {
			if h.Kind != domain.KindDocument {
				return fmt.Errorf("scoped search returned a %s hit", h.Kind)
			}
		}
		return nil
	})
	registerSlice4Steps(sc, w)
}

// registerSlice4Steps wires the step definitions for the @slice4 scenarios:
// linking (parent/related/blocking), labels, and the full issue view.
func registerSlice4Steps(sc *godog.ScenarioContext, w *world) {
	// --- Link issues ---
	sc.Step(`^two issues$`, func() error {
		var err error
		if w.linkA, err = w.create("Issue A", "body a"); err != nil {
			return err
		}
		w.linkB, err = w.create("Issue B", "body b")
		return err
	})
	sc.Step(`^I set one as another's parent_id$`, func() error {
		// Make B a child of A.
		_, w.err = w.runCLI("link", "parent", w.linkB.ID, "--parent", w.linkA.ID, "--json")
		return nil
	})
	sc.Step(`^the child and parent relation holds$`, func() error {
		if w.err != nil {
			return fmt.Errorf("set parent failed: %w", w.err)
		}
		child, err := w.verify.GetIssue(w.linkB.ID)
		if err != nil {
			return err
		}
		if child.ParentID == nil || *child.ParentID != w.linkA.ID {
			return fmt.Errorf("child parent_id = %v, want %q", child.ParentID, w.linkA.ID)
		}
		return nil
	})
	sc.Step(`^I set its parent_id to itself$`, func() error {
		_, w.err = w.runCLI("link", "parent", w.issue.ID, "--parent", w.issue.ID, "--json")
		return nil
	})
	sc.Step(`^issues A and B where B's parent is A$`, func() error {
		var err error
		if w.linkA, err = w.create("Issue A", "body a"); err != nil {
			return err
		}
		if w.linkB, err = w.create("Issue B", "body b"); err != nil {
			return err
		}
		_, err = w.runCLI("link", "parent", w.linkB.ID, "--parent", w.linkA.ID, "--json")
		return err
	})
	sc.Step(`^I set A's parent_id to B$`, func() error {
		_, w.err = w.runCLI("link", "parent", w.linkA.ID, "--parent", w.linkB.ID, "--json")
		return nil
	})
	sc.Step(`^it is rejected as a cycle$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected cycle rejection, got none")
		}
		return nil
	})
	sc.Step(`^issues A, B, and C forming a parent chain A to B to C$`, func() error {
		// A is parent of B, B is parent of C (A is the root ancestor).
		var err error
		if w.linkA, err = w.create("Issue A", "body a"); err != nil {
			return err
		}
		if w.linkB, err = w.create("Issue B", "body b"); err != nil {
			return err
		}
		if w.linkC, err = w.create("Issue C", "body c"); err != nil {
			return err
		}
		if _, err = w.runCLI("link", "parent", w.linkB.ID, "--parent", w.linkA.ID, "--json"); err != nil {
			return err
		}
		_, err = w.runCLI("link", "parent", w.linkC.ID, "--parent", w.linkB.ID, "--json")
		return err
	})
	sc.Step(`^I set A's parent_id to C$`, func() error {
		// C is a descendant of A, so making C the parent of A closes a cycle.
		_, w.err = w.runCLI("link", "parent", w.linkA.ID, "--parent", w.linkC.ID, "--json")
		return nil
	})
	sc.Step(`^I mark A as blocked by B$`, func() error {
		_, w.err = w.runCLI("link", "blocked", w.linkA.ID, "--by", w.linkB.ID, "--json")
		return w.err
	})
	sc.Step(`^A shows B in blocked_by and B shows A in is_blocking, derived from issue_block$`, func() error {
		a, err := w.verify.GetIssueView(w.linkA.ID)
		if err != nil {
			return err
		}
		if !containsStr(a.BlockedBy, w.linkB.ID) {
			return fmt.Errorf("A.blocked_by = %v, want to contain %q", a.BlockedBy, w.linkB.ID)
		}
		b, err := w.verify.GetIssueView(w.linkB.ID)
		if err != nil {
			return err
		}
		if !containsStr(b.IsBlocking, w.linkA.ID) {
			return fmt.Errorf("B.is_blocking = %v, want to contain %q", b.IsBlocking, w.linkA.ID)
		}
		return nil
	})
	sc.Step(`^I relate them$`, func() error {
		_, w.err = w.runCLI("link", "related", w.linkA.ID, "--to", w.linkB.ID, "--json")
		return w.err
	})
	sc.Step(`^each appears in the other's related list$`, func() error {
		a, err := w.verify.GetIssueView(w.linkA.ID)
		if err != nil {
			return err
		}
		if !containsStr(a.Related, w.linkB.ID) {
			return fmt.Errorf("A.related = %v, want to contain %q", a.Related, w.linkB.ID)
		}
		b, err := w.verify.GetIssueView(w.linkB.ID)
		if err != nil {
			return err
		}
		if !containsStr(b.Related, w.linkA.ID) {
			return fmt.Errorf("B.related = %v, want to contain %q", b.Related, w.linkA.ID)
		}
		return nil
	})

	// --- Label an issue ---
	sc.Step(`^I add a label$`, func() error {
		w.viewLabel = "backend"
		_, w.err = w.runCLI("label", "add", w.issue.ID, w.viewLabel, "--json")
		return w.err
	})
	sc.Step(`^it appears in labels$`, func() error {
		iss, err := w.verify.GetIssue(w.issue.ID)
		if err != nil {
			return err
		}
		if !containsStr(iss.Labels, w.viewLabel) {
			return fmt.Errorf("labels = %v, want to contain %q", iss.Labels, w.viewLabel)
		}
		return nil
	})
	sc.Step(`^an issue with a label$`, func() error {
		w.issue, w.err = w.create("Labelled", "body")
		if w.err != nil {
			return w.err
		}
		w.viewLabel = "frontend"
		_, w.err = w.runCLI("label", "add", w.issue.ID, w.viewLabel, "--json")
		return w.err
	})
	sc.Step(`^I remove the label$`, func() error {
		_, w.err = w.runCLI("label", "remove", w.issue.ID, w.viewLabel, "--json")
		return w.err
	})
	sc.Step(`^it no longer appears in labels$`, func() error {
		iss, err := w.verify.GetIssue(w.issue.ID)
		if err != nil {
			return err
		}
		if containsStr(iss.Labels, w.viewLabel) {
			return fmt.Errorf("labels = %v, still contains removed %q", iss.Labels, w.viewLabel)
		}
		return nil
	})

	// --- View an issue (full) ---
	sc.Step(`^an issue with comments, labels, and links$`, func() error {
		w.issue, w.err = w.create("Full view", "the body to read back")
		if w.err != nil {
			return w.err
		}
		// A comment.
		w.viewComment = "a considered comment"
		if _, err := w.runCLI("comment", w.issue.ID, "--body", w.viewComment, "--json"); err != nil {
			return err
		}
		// A label.
		w.viewLabel = "urgent"
		if _, err := w.runCLI("label", "add", w.issue.ID, w.viewLabel, "--json"); err != nil {
			return err
		}
		// A related link and a blocking link to two other issues.
		related, err := w.create("Related issue", "body")
		if err != nil {
			return err
		}
		w.viewRelated = related.ID
		if _, err := w.runCLI("link", "related", w.issue.ID, "--to", w.viewRelated, "--json"); err != nil {
			return err
		}
		blocker, err := w.create("Blocker issue", "body")
		if err != nil {
			return err
		}
		w.viewBlocker = blocker.ID
		_, err = w.runCLI("link", "blocked", w.issue.ID, "--by", w.viewBlocker, "--json")
		return err
	})
	sc.Step(`^its fields, labels, related and blocking links, and comments are shown$`, func() error {
		if w.err != nil {
			return fmt.Errorf("view failed: %w", w.err)
		}
		for label, want := range map[string]string{
			"id":         w.issue.ID,
			"subject":    w.issue.Subject,
			"body":       w.issue.Body,
			"status":     string(w.issue.Status),
			"label":      w.viewLabel,
			"related":    w.viewRelated,
			"blocked_by": w.viewBlocker,
			"comment":    w.viewComment,
		} {
			if !strings.Contains(w.viewOut, want) {
				return fmt.Errorf("view output missing %s (%q):\n%s", label, want, w.viewOut)
			}
		}
		return nil
	})
}

// containsStr reports whether s contains v.
func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
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
	sc.Step(`^deleted_at is set and it drops from default lists$`, func() error {
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
		// I1: bug, open, p1, owner agent-alpha
		i1, err := w.create("bug open", "body")
		if err != nil {
			return err
		}
		if i1, err = w.updateIssue(i1.ID, "--type", "bug", "--priority", "p1", "--owner", "agent-alpha"); err != nil {
			return err
		}
		// I2: bug, in_progress, p2 (default), owner agent-alpha
		i2, err := w.create("bug in progress", "body")
		if err != nil {
			return err
		}
		if i2, err = w.updateIssue(i2.ID, "--type", "bug", "--status", "in_progress", "--owner", "agent-alpha"); err != nil {
			return err
		}
		// I3: task, open, p3, owner agent-beta
		i3, err := w.create("task open", "body")
		if err != nil {
			return err
		}
		if i3, err = w.updateIssue(i3.ID, "--priority", "p3", "--owner", "agent-beta"); err != nil {
			return err
		}
		w.issues = []domain.Issue{i1, i2, i3}
		return nil
	})
	sc.Step(`^I list with a filter such as status=open, label=bug, or owner=AGENT$`, func() error {
		// Label filtering arrives with labels in slice 4; exercise S3-supported
		// fields here (status, priority, owner).
		w.listOut, w.err = w.listIssues("--status", "open")
		return w.err
	})
	sc.Step(`^only matching, non-soft-deleted issues are returned$`, func() error {
		// status=open must return exactly I1 and I3, never I2 (in_progress).
		if err := requireExactIssues(w.listOut, w.issues[0].ID, w.issues[2].ID); err != nil {
			return fmt.Errorf("status=open filter: %w", err)
		}
		// priority=p1 must return exactly I1.
		byPrio, err := w.listIssues("--priority", "p1")
		if err != nil {
			return err
		}
		if err := requireExactIssues(byPrio, w.issues[0].ID); err != nil {
			return fmt.Errorf("priority=p1 filter: %w", err)
		}
		// owner=agent-beta must return exactly I3.
		byOwner, err := w.listIssues("--owner", "agent-beta")
		if err != nil {
			return err
		}
		if err := requireExactIssues(byOwner, w.issues[2].ID); err != nil {
			return fmt.Errorf("owner filter: %w", err)
		}
		for _, i := range w.listOut {
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
		// type=bug AND status=open must return exactly I1.
		if err := requireExactIssues(w.listOut, w.issues[0].ID); err != nil {
			return fmt.Errorf("type=bug AND status=open: %w", err)
		}
		return nil
	})
}

// requireExactIssues asserts got contains exactly the wantIDs, no more, no less.
func requireExactIssues(got []domain.Issue, wantIDs ...string) error {
	if len(got) != len(wantIDs) {
		return fmt.Errorf("got %d issues, want %d", len(got), len(wantIDs))
	}
	for _, id := range wantIDs {
		if !containsIssue(got, id) {
			return fmt.Errorf("missing expected issue %s", id)
		}
	}
	return nil
}
