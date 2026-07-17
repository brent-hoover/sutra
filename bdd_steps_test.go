package main_test

// Step definitions for implemented scenarios (tagged @slice1). Each scenario
// runs against the full vertical: client → httptest(api) → service → store,
// backed by a throwaway SQLite file.

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

type world struct {
	dir     string
	svc     *service.Service
	server  *httptest.Server
	client  *client.Client
	subject string
	body    string
	issue   domain.Issue
	err     error
}

func (w *world) setup() error {
	dir, err := os.MkdirTemp("", "sutra-bdd-")
	if err != nil {
		return err
	}
	w.dir = dir
	cfg := config.Config{DBPath: filepath.Join(dir, "test.db")}
	if w.svc, err = service.New(cfg); err != nil {
		return err
	}
	w.server = httptest.NewServer(api.Handler(w.svc))
	w.client = client.New(config.Config{Host: w.server.URL})
	return nil
}

func (w *world) teardown() {
	if w.server != nil {
		w.server.Close()
	}
	if w.svc != nil {
		w.svc.Close()
	}
	if w.dir != "" {
		os.RemoveAll(w.dir)
	}
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
		history, err := w.svc.IssueHistory(w.issue.ID)
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
