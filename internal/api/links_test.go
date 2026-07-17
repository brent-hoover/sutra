package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/brent-hoover/sutra/internal/domain"
)

// createIssue posts a new issue to the test server and returns its id.
func createIssue(t *testing.T, srv *httptest.Server, subject string) string {
	t.Helper()
	resp, err := http.Post(srv.URL+"/issues", "application/json",
		strings.NewReader(`{"subject":"`+subject+`","body":"b"}`))
	if err != nil {
		t.Fatalf("POST /issues: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var iss domain.Issue
	if err := json.NewDecoder(resp.Body).Decode(&iss); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	return iss.ID
}

func do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// The linking and label endpoints return 400 for invalid input, 404 for a
// missing issue, and 200 with the updated issue on success.
func TestLinkAndLabelStatusCodes(t *testing.T) {
	srv := newTestServer(t)
	a := createIssue(t, srv, "A")
	b := createIssue(t, srv, "B")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"self-parent rejected", http.MethodPut, "/issues/" + a + "/parent", `{"parent_id":"` + a + `"}`, http.StatusBadRequest},
		{"parent missing issue", http.MethodPut, "/issues/nope/parent", `{"parent_id":"` + a + `"}`, http.StatusNotFound},
		{"set parent ok", http.MethodPut, "/issues/" + b + "/parent", `{"parent_id":"` + a + `"}`, http.StatusOK},
		{"relate ok", http.MethodPost, "/issues/" + a + "/relations", `{"related_issue_id":"` + b + `"}`, http.StatusOK},
		{"block ok", http.MethodPost, "/issues/" + a + "/blocks", `{"blocker_id":"` + b + `"}`, http.StatusOK},
		{"add label ok", http.MethodPost, "/issues/" + a + "/labels", `{"label":"backend"}`, http.StatusOK},
		{"empty label rejected", http.MethodPost, "/issues/" + a + "/labels", `{"label":""}`, http.StatusBadRequest},
		{"label missing issue", http.MethodPost, "/issues/nope/labels", `{"label":"x"}`, http.StatusNotFound},
		{"remove label ok", http.MethodDelete, "/issues/" + a + "/labels?label=backend", "", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, tc.method, srv.URL+tc.path, tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("status = %d, want %d (body: %s)", resp.StatusCode, tc.want, body)
			}
		})
	}
}

// GET /issues/{id} returns the full view: fields, labels, related/blocking
// links, and comments.
func TestGetIssueReturnsFullView(t *testing.T) {
	srv := newTestServer(t)
	a := createIssue(t, srv, "Full")
	b := createIssue(t, srv, "Related")

	do(t, http.MethodPost, srv.URL+"/issues/"+a+"/labels", `{"label":"urgent"}`).Body.Close()
	do(t, http.MethodPost, srv.URL+"/issues/"+a+"/relations", `{"related_issue_id":"`+b+`"}`).Body.Close()
	do(t, http.MethodPost, srv.URL+"/issues/"+a+"/blocks", `{"blocker_id":"`+b+`"}`).Body.Close()
	do(t, http.MethodPost, srv.URL+"/issues/"+a+"/comments", `{"body":"a note"}`).Body.Close()

	resp := do(t, http.MethodGet, srv.URL+"/issues/"+a, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", resp.StatusCode)
	}
	var view domain.IssueView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if len(view.Labels) != 1 || view.Labels[0] != "urgent" {
		t.Errorf("labels = %v, want [urgent]", view.Labels)
	}
	if len(view.Related) != 1 || view.Related[0] != b {
		t.Errorf("related = %v, want [%s]", view.Related, b)
	}
	if len(view.BlockedBy) != 1 || view.BlockedBy[0] != b {
		t.Errorf("blocked_by = %v, want [%s]", view.BlockedBy, b)
	}
	if len(view.Comments) != 1 || view.Comments[0].Body != "a note" {
		t.Errorf("comments = %v, want one comment 'a note'", view.Comments)
	}
}

// An issue with no links or comments still renders those fields as JSON arrays
// (never null), per the view contract.
func TestGetIssueEmptyCollectionsAreArrays(t *testing.T) {
	srv := newTestServer(t)
	a := createIssue(t, srv, "Bare")

	resp := do(t, http.MethodGet, srv.URL+"/issues/"+a, "")
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	body := string(raw)
	for _, want := range []string{`"related":[]`, `"blocked_by":[]`, `"is_blocking":[]`, `"comments":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("view body missing %s:\n%s", want, body)
		}
	}
}

// A free-text label like "." must survive add + remove — it travels as a query
// parameter, not a path segment (path canonicalization would strip ".").
func TestDotSegmentLabelRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	id := createIssue(t, srv, "dot label host")

	resp, err := http.Post(srv.URL+"/issues/"+id+"/labels", "application/json", strings.NewReader(`{"label":"."}`))
	if err != nil {
		t.Fatalf("add label: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add '.' label status = %d, want 200", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/issues/"+id+"/labels?label="+url.QueryEscape("."), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("remove label: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("remove '.' label status = %d, want 200", resp.StatusCode)
	}
	var view domain.IssueView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, l := range view.Labels {
		if l == "." {
			t.Errorf("label '.' still present after removal")
		}
	}
}
