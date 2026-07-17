package main_test

// Step definitions for the @slice2 transport-contract scenarios: every
// operation is reachable as JSON over HTTP, the CLI is a thin JSON client, and
// bearer-token auth gates LAN access. The harness runs with SUTRA_TOKEN set, so
// the daemon enforces auth and the client sends it.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/domain"
)

func registerSlice2Steps(sc *godog.ScenarioContext, w *world) {
	// per-scenario state
	var (
		httpStatus    int
		httpCT        string
		httpBody      []byte
		cliJSON       string
		noTokenStatus int
	)

	// rawGet issues a GET to the daemon with the given bearer token (empty = none).
	rawGet := func(path, token string) (int, string, []byte, error) {
		req, err := http.NewRequest(http.MethodGet, w.cfg.Host+path, nil)
		if err != nil {
			return 0, "", nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, "", nil, err
		}
		defer resp.Body.Close()
		buf, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type"), buf, nil
	}

	// --- JSON API covers every operation ---
	sc.Step(`^the running daemon$`, func() error { return nil })
	sc.Step(`^any operation from the other epics is invoked over HTTP$`, func() error {
		// Create an issue directly over HTTP (with the configured token).
		req, err := http.NewRequest(http.MethodPost, w.cfg.Host+"/issues", strings.NewReader(`{"subject":"over http","body":"b"}`))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+w.cfg.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		httpStatus = resp.StatusCode
		httpCT = resp.Header.Get("Content-Type")
		httpBody, _ = io.ReadAll(resp.Body)
		return nil
	})
	sc.Step(`^it accepts and returns JSON with one endpoint per operation$`, func() error {
		if httpStatus != http.StatusCreated {
			return fmt.Errorf("status = %d, want 201", httpStatus)
		}
		if !strings.HasPrefix(httpCT, "application/json") {
			return fmt.Errorf("content-type = %q, want application/json", httpCT)
		}
		var issue domain.Issue
		if err := json.Unmarshal(httpBody, &issue); err != nil {
			return fmt.Errorf("response is not JSON: %w", err)
		}
		if issue.ID == "" {
			return fmt.Errorf("JSON response missing id")
		}
		return nil
	})

	// --- CLI is a thin client ---
	sc.Step(`^the daemon is running$`, func() error { return nil })
	sc.Step(`^I run a CLI command$`, func() error {
		cliJSON, w.err = w.runCLI("create", "--subject", "thin client", "--body", "b", "--json")
		return w.err
	})
	sc.Step(`^it calls exactly one API endpoint and holds no behavior the API doesn't expose$`, func() error {
		// The CLI output is the API's JSON response verbatim — it added nothing.
		if w.err != nil {
			return w.err
		}
		if !json.Valid([]byte(strings.TrimSpace(cliJSON))) {
			return fmt.Errorf("CLI output is not the raw API JSON: %q", cliJSON)
		}
		return nil
	})
	sc.Step(`^the json flag$`, func() error { return nil })
	sc.Step(`^it prints the API's raw JSON response$`, func() error {
		var issue domain.Issue
		if err := json.Unmarshal([]byte(strings.TrimSpace(cliJSON)), &issue); err != nil {
			return fmt.Errorf("output not raw JSON: %w", err)
		}
		if issue.Subject != "thin client" {
			return fmt.Errorf("subject = %q, want %q", issue.Subject, "thin client")
		}
		return nil
	})

	// --- Authenticated LAN access ---
	sc.Step(`^a configured bearer token$`, func() error {
		if w.cfg.Token == "" {
			return fmt.Errorf("expected a configured bearer token")
		}
		return nil
	})
	sc.Step(`^a client sends a request with the correct token$`, func() error {
		var err error
		httpStatus, _, _, err = rawGet("/issues", w.cfg.Token)
		return err
	})
	sc.Step(`^it succeeds$`, func() error {
		if httpStatus != http.StatusOK {
			return fmt.Errorf("status = %d, want 200", httpStatus)
		}
		return nil
	})
	sc.Step(`^a wrong or missing token$`, func() error { return nil })
	sc.Step(`^a client sends a request$`, func() error {
		var err error
		noTokenStatus, _, _, err = rawGet("/issues", "") // no Authorization header
		if err != nil {
			return err
		}
		wrong, _, _, err := rawGet("/issues", "not-the-token")
		if err != nil {
			return err
		}
		if wrong != http.StatusUnauthorized {
			return fmt.Errorf("wrong-token status = %d, want 401", wrong)
		}
		return nil
	})
	sc.Step(`^it is rejected with 401$`, func() error {
		if noTokenStatus != http.StatusUnauthorized {
			return fmt.Errorf("missing-token status = %d, want 401", noTokenStatus)
		}
		return nil
	})

	// --- Use clients from another machine ---
	sc.Step(`^SUTRA_HOST set to the daemon's LAN address and a valid token$`, func() error {
		if w.cfg.Host == "" || w.cfg.Token == "" {
			return fmt.Errorf("expected SUTRA_HOST and token to be set")
		}
		return nil
	})
	sc.Step(`^I run the CLI or TUI from another machine$`, func() error {
		// The client targets the daemon via SUTRA_HOST with the token, exactly as
		// a different machine would.
		w.createIssue("from afar", "created over the network")
		return w.err
	})
	sc.Step(`^it operates against the daemon's data$`, func() error {
		if w.err != nil {
			return w.err
		}
		// Confirm it persisted in the daemon's store (read via the verify handle).
		if _, err := w.verify.GetIssue(w.issue.ID); err != nil {
			return fmt.Errorf("issue not persisted in daemon store: %w", err)
		}
		return nil
	})
}
