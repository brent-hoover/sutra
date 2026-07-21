package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/brent-hoover/sutra/internal/domain"
)

// PlanStepInput is one tracer item sent to the daemon when building a plan.
// Body, Type, and Priority are optional (empty is omitted, so the daemon applies
// its defaults).
type PlanStepInput struct {
	Subject  string `json:"subject"`
	Body     string `json:"body,omitempty"`
	Type     string `json:"type,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// PlanResult carries the built plan issue, its tracer children, and the raw JSON
// response body.
type PlanResult struct {
	Plan     domain.Issue
	Children []domain.Issue
	Raw      json.RawMessage
}

// BuildPlan builds a plan issue and its tracer children in one request. parentID
// and projectID are optional (empty is omitted).
func (c *Client) BuildPlan(ctx context.Context, title, prose string, steps []PlanStepInput, parentID, projectID string) (PlanResult, error) {
	payload := map[string]any{"title": title, "prose": prose, "steps": steps}
	if parentID != "" {
		payload["parent_id"] = parentID
	}
	if projectID != "" {
		payload["project_id"] = projectID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return PlanResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/plans", bytes.NewReader(body))
	if err != nil {
		return PlanResult{}, err
	}
	raw, err := c.do(req, http.StatusCreated)
	if err != nil {
		return PlanResult{}, err
	}
	var resp struct {
		Plan     domain.Issue   `json:"plan"`
		Children []domain.Issue `json:"children"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return PlanResult{}, err
	}
	return PlanResult{Plan: resp.Plan, Children: resp.Children, Raw: raw}, nil
}

// ApprovePlan approves a plan issue via the daemon.
func (c *Client) ApprovePlan(ctx context.Context, id string) (IssueResult, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(id)+"/plan/approve", nil)
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}
