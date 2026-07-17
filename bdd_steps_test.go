package main_test

// Step definitions for implemented scenarios (tagged @slice1). Each scenario
// runs against the real serve path: a live api.Run daemon on a loopback
// listener, exercised through the HTTP client, backed by a throwaway SQLite
// file. A second service handle on the same file verifies ledger writes.

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

type world struct {
	dir     string
	verify  *service.Service // second handle for reading ledger state
	serveCh chan error       // receives api.Run's exit error
	client  *client.Client
	subject string
	body    string
	issue   domain.Issue
	viewed  domain.Issue
	err     error
}

// freeLoopbackAddr returns an unused 127.0.0.1:port for the daemon to bind.
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
	dbPath := filepath.Join(dir, "test.db")

	addr, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	cfg := config.Config{ListenAddr: addr, DBPath: dbPath, Host: "http://" + addr}

	// Start the real daemon (config + api.Run + store open + listen).
	w.serveCh = make(chan error, 1)
	go func() { w.serveCh <- api.Run(cfg) }()
	if err := w.waitListening(addr); err != nil {
		return err
	}

	w.client = client.New(cfg)
	if w.verify, err = service.New(config.Config{DBPath: dbPath}); err != nil {
		return err
	}
	return nil
}

func (w *world) waitListening(addr string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-w.serveCh:
			return fmt.Errorf("daemon exited before listening: %w", err)
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

func (w *world) teardown() {
	if w.verify != nil {
		w.verify.Close()
	}
	if w.dir != "" {
		os.RemoveAll(w.dir)
	}
	// The api.Run goroutine has no shutdown handle; it is left idle and reaped
	// when the test binary exits. Each scenario binds a fresh port.
}

// InitializeScenario wires a fresh world per scenario and registers steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	w := &world{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, w.setup()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.teardown()
		return ctx, nil
	})

	// Create an issue
	sc.Step(`^no other input$`, func() error { return nil })
	sc.Step(`^I create an issue with a subject and body$`, func(ctx context.Context) error {
		w.issue, w.err = w.client.CreateIssue(ctx, "Skeleton subject", "Skeleton body")
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
	sc.Step(`^a new issue is created$`, func(ctx context.Context) error {
		if w.issue.ID == "" {
			w.issue, w.err = w.client.CreateIssue(ctx, "Skeleton subject", "Skeleton body")
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
	sc.Step(`^a missing subject or body$`, func() error {
		w.subject, w.body = "Has a subject", ""
		return nil
	})
	sc.Step(`^I try to create the issue$`, func(ctx context.Context) error {
		w.issue, w.err = w.client.CreateIssue(ctx, w.subject, w.body)
		return nil
	})
	sc.Step(`^it is rejected$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected creation to be rejected, got none")
		}
		return nil
	})

	// View an issue's core fields
	sc.Step(`^a stored issue exists$`, func(ctx context.Context) error {
		w.issue, w.err = w.client.CreateIssue(ctx, "View me", "the body to read back")
		return w.err
	})
	sc.Step(`^I view it by id$`, func(ctx context.Context) error {
		w.viewed, w.err = w.client.GetIssue(ctx, w.issue.ID)
		return nil
	})
	sc.Step(`^its core fields are returned$`, func() error {
		if w.err != nil {
			return fmt.Errorf("view failed: %w", w.err)
		}
		if w.viewed.ID != w.issue.ID || w.viewed.Subject != w.issue.Subject || w.viewed.Body != w.issue.Body {
			return fmt.Errorf("viewed issue does not match created issue")
		}
		if w.viewed.Type != w.issue.Type || w.viewed.Status != w.issue.Status || w.viewed.Priority != w.issue.Priority {
			return fmt.Errorf("viewed issue metadata does not match")
		}
		return nil
	})

	// Run the daemon
	sc.Step(`^a config with a listen address and DB path$`, func() error { return nil })
	sc.Step(`^I run sutra serve$`, func() error { return nil })
	sc.Step(`^it opens the store and serves HTTP on that address$`, func(ctx context.Context) error {
		issue, err := w.client.CreateIssue(ctx, "Ping", "Prove the daemon serves and the store is open")
		if err != nil {
			return fmt.Errorf("daemon did not serve a working request: %w", err)
		}
		if _, err := w.client.GetIssue(ctx, issue.ID); err != nil {
			return fmt.Errorf("daemon did not read back from the store: %w", err)
		}
		return nil
	})
}
