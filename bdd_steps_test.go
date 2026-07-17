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
	dir         string
	cfg         config.Config
	verify      *service.Service // second handle for reading ledger state
	serveCtx    context.Context
	cancel      context.CancelFunc
	serveCh     chan error // receives the serve command's exit error
	serveExited bool       // set once the serve exit has been observed
	serveErr    error      // the observed serve exit error, if any
	subject     string
	body        string
	issue       domain.Issue
	viewOut     string
	err         error
}

func freeLoopbackAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}

func (w *world) setup() error {
	dir, err := os.MkdirTemp("", "sutra-bdd-")
	if err != nil {
		return err
	}
	w.dir = dir

	addr, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	// Exercise config.Load through the environment.
	os.Setenv("SUTRA_LISTEN", addr)
	os.Setenv("SUTRA_DB", filepath.Join(dir, "test.db"))
	os.Setenv("SUTRA_HOST", "http://"+addr)
	os.Unsetenv("SUTRA_TOKEN")
	w.cfg = config.Load()

	// Start the daemon via the real Cobra `serve` command.
	w.serveCtx, w.cancel = context.WithCancel(context.Background())
	w.serveCh = make(chan error, 1)
	go func() {
		root := cli.NewRoot(w.cfg)
		root.SetArgs([]string{"serve"})
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		w.serveCh <- root.ExecuteContext(w.serveCtx)
	}()
	if err := w.waitListening(addr); err != nil {
		return err
	}

	if w.verify, err = service.New(config.Config{DBPath: w.cfg.DBPath}); err != nil {
		return err
	}
	return nil
}

func (w *world) waitListening(addr string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-w.serveCh:
			w.serveExited, w.serveErr = true, err
			return fmt.Errorf("serve command exited before listening: %w", err)
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start listening on %s", addr)
}

func (w *world) teardown() error {
	os.Unsetenv("SUTRA_LISTEN")
	os.Unsetenv("SUTRA_DB")
	os.Unsetenv("SUTRA_HOST")

	var errs []error
	shutdownConfirmed := w.cancel == nil
	if w.cancel != nil {
		w.cancel()
		switch {
		case w.serveExited:
			// The daemon already exited (observed during setup); don't block.
			if w.serveErr != nil {
				errs = append(errs, fmt.Errorf("serve exited with error: %w", w.serveErr))
			}
			shutdownConfirmed = true
		default:
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
