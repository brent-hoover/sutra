package client_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

func newPlanClient(t *testing.T) *client.Client {
	t.Helper()
	dir := t.TempDir()
	svc, err := service.New(config.Config{DBPath: filepath.Join(dir, "t.db"), ProjectsDir: filepath.Join(dir, "projects")})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	srv := httptest.NewServer(api.Handler(svc))
	t.Cleanup(srv.Close)
	return client.New(config.Config{Host: srv.URL})
}

func TestClientBuildAndApprovePlan(t *testing.T) {
	c := newPlanClient(t)
	ctx := context.Background()

	res, err := c.BuildPlan(ctx, "Search", "overall prose",
		[]client.PlanStepInput{{Subject: "a"}, {Subject: "b"}}, "", "")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if res.Plan.Type != domain.TypePlan || res.Plan.Approval != domain.ApprovalPending {
		t.Errorf("plan type/approval = %q/%q", res.Plan.Type, res.Plan.Approval)
	}
	if len(res.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(res.Children))
	}

	got, err := c.ApprovePlan(ctx, res.Plan.ID)
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if got.Issue.Approval != domain.ApprovalApproved {
		t.Errorf("approval = %q, want approved", got.Issue.Approval)
	}
}
