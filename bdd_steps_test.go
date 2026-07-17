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
		abs, _ := filepath.Abs(w.sessionPath)
		if w.transcript.SourcePath != abs {
			return fmt.Errorf("source_path = %q, want %q", w.transcript.SourcePath, abs)
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
}
