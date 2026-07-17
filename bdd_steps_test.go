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
}
