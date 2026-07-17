package main_test

// Step definitions for the @slice2 transport-contract scenarios: every
// operation is reachable as JSON over HTTP, the CLI is a thin JSON client that
// makes exactly one request and prints the response verbatim, and bearer-token
// auth gates LAN access. The harness runs with SUTRA_TOKEN set, so the daemon
// enforces auth and the client sends it.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

func registerSlice2Steps(sc *godog.ScenarioContext, w *world) {
	var (
		httpStatus    int
		cliJSON       string
		cliReqs       int64
		noTokenStatus int
	)

	// req sends an authorized request to the daemon and returns status,
	// content-type, and body.
	req := func(method, path, body, token string) (int, string, []byte, error) {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		rq, err := http.NewRequest(method, w.cfg.Host+path, r)
		if err != nil {
			return 0, "", nil, err
		}
		if body != "" {
			rq.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			rq.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(rq)
		if err != nil {
			return 0, "", nil, err
		}
		defer resp.Body.Close()
		buf, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type"), buf, nil
	}

	// runCLICounted runs the CLI against an instrumented server (fresh store)
	// that counts requests, so we can assert the CLI made exactly one.
	runCLICounted := func(args ...string) (string, int64, error) {
		dir, err := os.MkdirTemp("", "sutra-thin-")
		if err != nil {
			return "", 0, err
		}
		defer os.RemoveAll(dir)
		svc, err := service.New(config.Config{DBPath: filepath.Join(dir, "t.db"), ProjectsDir: filepath.Join(dir, "projects")})
		if err != nil {
			return "", 0, err
		}
		defer svc.Close()
		var count int64
		h := api.HandlerWithAuth(svc, w.cfg.Token)
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&count, 1)
			h.ServeHTTP(rw, r)
		}))
		defer srv.Close()
		saved := w.cfg.Host
		w.cfg.Host = srv.URL
		defer func() { w.cfg.Host = saved }()
		out, err := w.runCLI(args...)
		return out, atomic.LoadInt64(&count), err
	}

	// --- JSON API covers every operation ---
	sc.Step(`^the running daemon$`, func() error { return nil })
	sc.Step(`^any operation from the other epics is invoked over HTTP$`, func() error {
		// Create, then read back across several endpoints spanning the epics.
		st, ct, body, err := req(http.MethodPost, "/issues", `{"subject":"over http","body":"b"}`, w.cfg.Token)
		if err != nil {
			return err
		}
		if st != http.StatusCreated || !strings.HasPrefix(ct, "application/json") {
			return fmt.Errorf("create: status=%d ct=%q", st, ct)
		}
		var iss domain.Issue
		if err := json.Unmarshal(body, &iss); err != nil {
			return fmt.Errorf("create not JSON: %w", err)
		}
		// Each of these must respond with JSON too.
		for _, op := range []struct{ method, path string }{
			{http.MethodGet, "/issues"},
			{http.MethodGet, "/issues/" + iss.ID},
			{http.MethodGet, "/issues/" + iss.ID + "/history"},
			{http.MethodGet, "/issues/" + iss.ID + "/documents"},
			{http.MethodGet, "/search?q=over"},
		} {
			st, ct, b, err := req(op.method, op.path, "", w.cfg.Token)
			if err != nil {
				return err
			}
			if st != http.StatusOK || !strings.HasPrefix(ct, "application/json") {
				return fmt.Errorf("%s %s: status=%d ct=%q", op.method, op.path, st, ct)
			}
			if !json.Valid(b) {
				return fmt.Errorf("%s %s: body not JSON", op.method, op.path)
			}
		}
		return nil
	})
	sc.Step(`^it accepts and returns JSON with one endpoint per operation$`, func() error {
		return nil // assertions performed in the When above
	})

	// --- CLI is a thin client ---
	sc.Step(`^the daemon is running$`, func() error { return nil })
	sc.Step(`^I run a CLI command$`, func() error {
		cliJSON, cliReqs, w.err = runCLICounted("create", "--subject", "thin client", "--body", "b", "--json")
		return w.err
	})
	sc.Step(`^it calls exactly one API endpoint and holds no behavior the API doesn't expose$`, func() error {
		if w.err != nil {
			return w.err
		}
		if cliReqs != 1 {
			return fmt.Errorf("CLI made %d requests, want exactly 1", cliReqs)
		}
		if !json.Valid([]byte(strings.TrimSpace(cliJSON))) {
			return fmt.Errorf("CLI output is not the raw API JSON: %q", cliJSON)
		}
		return nil
	})
	sc.Step(`^the json flag$`, func() error { return nil })
	sc.Step(`^it prints the API's raw JSON response$`, func() error {
		var iss domain.Issue
		if err := json.Unmarshal([]byte(strings.TrimSpace(cliJSON)), &iss); err != nil {
			return fmt.Errorf("output not raw JSON: %w", err)
		}
		if iss.Subject != "thin client" {
			return fmt.Errorf("subject = %q, want %q", iss.Subject, "thin client")
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
		httpStatus, _, _, err = req(http.MethodGet, "/issues", "", w.cfg.Token)
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
		if noTokenStatus, _, _, err = req(http.MethodGet, "/issues", "", ""); err != nil {
			return err
		}
		wrong, _, _, err := req(http.MethodGet, "/issues", "", "not-the-token")
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
		w.createIssue("from afar", "created over the network")
		return w.err
	})
	sc.Step(`^it operates against the daemon's data$`, func() error {
		if w.err != nil {
			return w.err
		}
		if _, err := w.verify.GetIssue(w.issue.ID); err != nil {
			return fmt.Errorf("issue not persisted in daemon store: %w", err)
		}
		return nil
	})
}
