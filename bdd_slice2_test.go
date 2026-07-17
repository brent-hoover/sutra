package main_test

// Step definitions for the @slice2 transport-contract scenarios: every
// registered operation is reachable as JSON over HTTP; the CLI is a thin JSON
// client that makes exactly one request and prints the response byte-for-byte;
// bearer-token auth gates LAN access; and a client honors a configured remote
// endpoint. The harness runs with SUTRA_TOKEN set, so the daemon enforces auth
// and the client sends it.

import (
	"context"
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
	"github.com/brent-hoover/sutra/internal/service"
)

func registerSlice2Steps(sc *godog.ScenarioContext, w *world) {
	var (
		httpStatus    int
		cliOut        string
		cliReqs       int64
		noTokenStatus int
		remoteHits    int64
		remoteIssueID string
		remoteSvc     *service.Service
		remoteSrv     *httptest.Server
		remoteDir     string
	)

	// Clean up the "remote machine" fixtures (if the scenario created them).
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if remoteSrv != nil {
			remoteSrv.Close()
		}
		if remoteSvc != nil {
			remoteSvc.Close()
		}
		if remoteDir != "" {
			os.RemoveAll(remoteDir)
		}
		return ctx, nil
	})

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

	// mustJSON asserts an endpoint returns a 2xx JSON response.
	mustJSON := func(method, path, body string) ([]byte, error) {
		st, ct, b, err := req(method, path, body, w.cfg.Token)
		if err != nil {
			return nil, err
		}
		if st < 200 || st >= 300 {
			return nil, fmt.Errorf("%s %s: status=%d body=%s", method, path, st, b)
		}
		if !strings.HasPrefix(ct, "application/json") {
			return nil, fmt.Errorf("%s %s: content-type=%q, want application/json", method, path, ct)
		}
		if !json.Valid(b) {
			return nil, fmt.Errorf("%s %s: body is not JSON: %s", method, path, b)
		}
		return b, nil
	}
	idOf := func(b []byte) string {
		var v struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(b, &v)
		return v.ID
	}

	// --- JSON API covers every operation ---
	sc.Step(`^the running daemon$`, func() error { return nil })
	sc.Step(`^any operation from the other epics is invoked over HTTP$`, func() error {
		a, err := mustJSON(http.MethodPost, "/issues", `{"subject":"op cover","body":"b"}`)
		if err != nil {
			return err
		}
		other, err := mustJSON(http.MethodPost, "/issues", `{"subject":"other","body":"b"}`)
		if err != nil {
			return err
		}
		aID, bID := idOf(a), idOf(other)

		// A transcript fixture under the daemon's projects dir, for ingest.
		sess := "cover000-0000-0000-0000-000000000001"
		fixtureDir := filepath.Join(w.cfg.ProjectsDir, "-cover")
		if err := os.MkdirAll(fixtureDir, 0o700); err != nil {
			return err
		}
		fixture := filepath.Join(fixtureDir, sess+".jsonl")
		if err := os.WriteFile(fixture, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0o600); err != nil {
			return err
		}

		docBody, err := mustJSON(http.MethodPost, "/issues/"+aID+"/documents", `{"kind":"problem","title":"t","content":"c"}`)
		if err != nil {
			return err
		}
		docID := idOf(docBody)
		trBody, err := mustJSON(http.MethodPost, "/transcripts", `{"source_path":"`+fixture+`"}`)
		if err != nil {
			return err
		}
		trID := idOf(trBody)

		// Every remaining registered route must answer with JSON.
		steps := []struct{ method, path, body string }{
			{http.MethodGet, "/issues", ""},
			{http.MethodGet, "/issues/" + aID, ""},
			{http.MethodPatch, "/issues/" + aID, `{"status":"in_progress"}`},
			{http.MethodGet, "/issues/" + aID + "/history", ""},
			{http.MethodPost, "/issues/" + aID + "/comments", `{"body":"c"}`},
			{http.MethodGet, "/issues/" + aID + "/comments", ""},
			{http.MethodGet, "/issues/" + aID + "/documents", ""},
			{http.MethodGet, "/documents/" + docID, ""},
			{http.MethodPatch, "/documents/" + docID, `{"content":"c2"}`},
			{http.MethodPost, "/issues/" + aID + "/labels", `{"label":"urgent"}`},
			{http.MethodDelete, "/issues/" + aID + "/labels?label=urgent", ""},
			{http.MethodPut, "/issues/" + aID + "/parent", `{"parent_id":"` + bID + `"}`},
			{http.MethodPost, "/issues/" + aID + "/relations", `{"related_issue_id":"` + bID + `"}`},
			{http.MethodPost, "/issues/" + aID + "/blocks", `{"blocker_id":"` + bID + `"}`},
			{http.MethodGet, "/transcripts", ""},
			{http.MethodGet, "/transcripts/" + trID, ""},
			{http.MethodPost, "/transcripts/" + trID + "/link", `{"issue_id":"` + aID + `"}`},
			{http.MethodGet, "/issues/" + aID + "/transcripts", ""},
			{http.MethodGet, "/search?q=cover", ""},
			{http.MethodDelete, "/documents/" + docID, ""},
			{http.MethodDelete, "/issues/" + bID, ""},
		}
		for _, s := range steps {
			if _, err := mustJSON(s.method, s.path, s.body); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^it accepts and returns JSON with one endpoint per operation$`, func() error {
		return nil // asserted in the When above
	})

	// --- CLI is a thin client ---
	sc.Step(`^the daemon is running$`, func() error { return nil })
	sc.Step(`^I run a CLI command$`, func() error {
		// Point the CLI at an instrumented server that returns a KNOWN response
		// body and counts requests, so we can assert exactly one request and
		// byte-for-byte passthrough.
		const canned = `{"id":"canned-id","subject":"thin client","body":"b","type":"task","status":"open","priority":"p2","created_at":"2026-07-16T10:00:00Z","updated_at":"2026-07-16T10:00:00Z"}`
		var count int64
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&count, 1)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusCreated)
			_, _ = rw.Write([]byte(canned))
		}))
		defer srv.Close()
		saved := w.cfg.Host
		w.cfg.Host = srv.URL
		cliOut, w.err = w.runCLI("create", "--subject", "thin client", "--body", "b", "--json")
		w.cfg.Host = saved
		cliReqs = atomic.LoadInt64(&count)
		// Stash the canned expectation for the Then via cliOut comparison.
		if w.err == nil && strings.TrimSpace(cliOut) != canned {
			w.err = fmt.Errorf("CLI did not print the response verbatim:\n got: %q\nwant: %q", strings.TrimSpace(cliOut), canned)
		}
		return w.err
	})
	sc.Step(`^it calls exactly one API endpoint and holds no behavior the API doesn't expose$`, func() error {
		if w.err != nil {
			return w.err
		}
		if cliReqs != 1 {
			return fmt.Errorf("CLI made %d requests, want exactly 1", cliReqs)
		}
		return nil
	})
	sc.Step(`^the json flag$`, func() error { return nil })
	sc.Step(`^it prints the API's raw JSON response$`, func() error {
		// Verbatim equality was asserted in the When; confirm it parses too.
		if !json.Valid([]byte(strings.TrimSpace(cliOut))) {
			return fmt.Errorf("output not valid JSON: %q", cliOut)
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
		// Stand up a DISTINCT daemon (its own store) and point the client at it,
		// proving the client honors the configured SUTRA_HOST rather than a
		// hardcoded endpoint.
		var err error
		if remoteDir, err = os.MkdirTemp("", "sutra-remote-"); err != nil {
			return err
		}
		if remoteSvc, err = service.New(config.Config{DBPath: filepath.Join(remoteDir, "t.db"), ProjectsDir: filepath.Join(remoteDir, "projects")}); err != nil {
			return err
		}
		h := api.HandlerWithAuth(remoteSvc, w.cfg.Token)
		remoteSrv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&remoteHits, 1)
			h.ServeHTTP(rw, r)
		}))
		w.cfg.Host = remoteSrv.URL // the "remote" endpoint the client must honor
		return nil
	})
	sc.Step(`^I run the CLI or TUI from another machine$`, func() error {
		out, err := w.runCLI("create", "--subject", "from afar", "--body", "b", "--json")
		if err != nil {
			w.err = err
			return nil
		}
		remoteIssueID = idOf([]byte(strings.TrimSpace(out)))
		return nil
	})
	sc.Step(`^it operates against the daemon's data$`, func() error {
		if w.err != nil {
			return w.err
		}
		if remoteHits == 0 {
			return fmt.Errorf("the configured remote endpoint received no requests")
		}
		// The issue must live in the REMOTE daemon's store, proving the client
		// targeted the configured host.
		if _, err := remoteSvc.GetIssue(remoteIssueID); err != nil {
			return fmt.Errorf("issue not in the remote daemon's store: %w", err)
		}
		return nil
	})
}
