package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/brent-hoover/sutra/internal/domain"
)

func postPlan(t *testing.T, srv *httptest.Server, body string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/plans", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /plans: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, raw
}

func TestBuildPlanEndpoint(t *testing.T) {
	srv := newTestServer(t)
	resp, raw := postPlan(t, srv, `{"title":"Search","prose":"overall","steps":[{"subject":"a"},{"subject":"b"},{"subject":"c"}]}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}
	var got struct {
		Plan     domain.Issue   `json:"plan"`
		Children []domain.Issue `json:"children"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Plan.Type != domain.TypePlan || got.Plan.Approval != domain.ApprovalPending {
		t.Errorf("plan type/approval = %q/%q", got.Plan.Type, got.Plan.Approval)
	}
	if len(got.Children) != 3 {
		t.Errorf("children = %d, want 3", len(got.Children))
	}
}

func TestBuildPlanEndpointRejections(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name, body string
		want       int
	}{
		{"empty steps", `{"title":"t","prose":"p","steps":[]}`, http.StatusBadRequest},
		{"empty subject", `{"title":"t","prose":"p","steps":[{"subject":""}]}`, http.StatusBadRequest},
		{"invalid type", `{"title":"t","prose":"p","steps":[{"subject":"a","type":"epic"}]}`, http.StatusBadRequest},
		{"plan type", `{"title":"t","prose":"p","steps":[{"subject":"a","type":"plan"}]}`, http.StatusBadRequest},
		{"empty title", `{"title":"","prose":"p","steps":[{"subject":"a"}]}`, http.StatusBadRequest},
		{"missing parent", `{"title":"t","prose":"p","parent_id":"nope","steps":[{"subject":"a"}]}`, http.StatusNotFound},
		{"missing project", `{"title":"t","prose":"p","project_id":"nope","steps":[{"subject":"a"}]}`, http.StatusNotFound},
		{"unknown field", `{"title":"t","prose":"p","bogus":1,"steps":[{"subject":"a"}]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := postPlan(t, srv, tc.body)
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestListByParentEndpoint(t *testing.T) {
	srv := newTestServer(t)
	_, raw := postPlan(t, srv, `{"title":"t","prose":"p","steps":[{"subject":"a"},{"subject":"b"},{"subject":"c"}]}`)
	var built struct {
		Plan domain.Issue `json:"plan"`
	}
	if err := json.Unmarshal(raw, &built); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp, err := http.Get(srv.URL + "/issues?parent=" + built.Plan.ID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	var children []domain.Issue
	if err := json.Unmarshal(body, &children); err != nil {
		t.Fatalf("decode children: %v", err)
	}
	if len(children) != 3 {
		t.Errorf("children = %d, want 3", len(children))
	}
	for _, c := range children {
		if c.ParentID == nil || *c.ParentID != built.Plan.ID {
			t.Errorf("child %s parent = %v, want %s", c.ID, c.ParentID, built.Plan.ID)
		}
	}
}

func TestApprovePlanEndpoint(t *testing.T) {
	srv := newTestServer(t)
	_, raw := postPlan(t, srv, `{"title":"t","prose":"p","steps":[{"subject":"a"}]}`)
	var built struct {
		Plan domain.Issue `json:"plan"`
	}
	if err := json.Unmarshal(raw, &built); err != nil {
		t.Fatalf("decode built: %v", err)
	}

	// Approve the plan → 200, approved.
	resp, err := http.Post(srv.URL+"/issues/"+built.Plan.ID+"/plan/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d: %s", resp.StatusCode, body)
	}
	var approved domain.Issue
	if err := json.Unmarshal(body, &approved); err != nil {
		t.Fatalf("decode approved: %v", err)
	}
	if approved.Approval != domain.ApprovalApproved {
		t.Errorf("approval = %q, want approved", approved.Approval)
	}

	// Approve a non-plan issue → 400.
	task := postIssue(t, srv, `{"subject":"x","body":"y"}`)
	resp2, _ := http.Post(srv.URL+"/issues/"+task.ID+"/plan/approve", "application/json", nil)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("approve task status = %d, want 400", resp2.StatusCode)
	}

	// Approve a missing issue → 404.
	resp3, _ := http.Post(srv.URL+"/issues/missing/plan/approve", "application/json", nil)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("approve missing status = %d, want 404", resp3.StatusCode)
	}
}

func TestUpdateIssueRejectsPlanTypeTransitions(t *testing.T) {
	srv := newTestServer(t)
	patchType := func(id, typ string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPatch, srv.URL+"/issues/"+id, strings.NewReader(`{"type":"`+typ+`"}`))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PATCH: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	task := postIssue(t, srv, `{"subject":"x","body":"y"}`)
	if got := patchType(task.ID, "plan"); got != http.StatusBadRequest {
		t.Errorf("task -> plan status = %d, want 400", got)
	}

	_, raw := postPlan(t, srv, `{"title":"t","prose":"p","steps":[{"subject":"a"}]}`)
	var built struct {
		Plan domain.Issue `json:"plan"`
	}
	if err := json.Unmarshal(raw, &built); err != nil {
		t.Fatalf("decode built: %v", err)
	}
	resp, err := http.Post(srv.URL+"/issues/"+built.Plan.ID+"/plan/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d, want 200", resp.StatusCode)
	}
	if got := patchType(built.Plan.ID, "task"); got != http.StatusBadRequest {
		t.Errorf("plan -> task status = %d, want 400", got)
	}
}
