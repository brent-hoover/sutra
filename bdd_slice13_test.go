package main_test

// Step definitions for the @slice13 scenarios: building a plan issue tree
// (plan issue + tracer children + sequential blocking chain) and approving a
// plan. Each step drives the real `sutra plan …` CLI against the live daemon;
// stored state is verified through the second service handle.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/domain"
)

type planBuildResult struct {
	Plan     domain.Issue   `json:"plan"`
	Children []domain.Issue `json:"children"`
}

func registerSlice13Steps(sc *godog.ScenarioContext, w *world) {
	const (
		threeSteps = `[{"subject":"one","body":"b1"},{"subject":"two","body":"b2"},{"subject":"three","body":"b3"}]`
		oneStep    = `[{"subject":"only","body":"b"}]`
		twoSteps   = `[{"subject":"a","body":"ba"},{"subject":"b","body":"bb"}]`
	)
	var (
		planResp        planBuildResult
		pendingSteps    string
		featureID       string
		featProject     string
		explicitProject string
		taskID          string
		approved        domain.Issue
		ledgerBefore    int
	)

	createProject := func(name, repo string) (string, error) {
		out, err := w.runCLI("project", "create", "--name", name, "--repo", repo, "--json")
		if err != nil {
			return "", err
		}
		var p domain.Project
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &p); err != nil {
			return "", err
		}
		return p.ID, nil
	}

	build := func(title, steps, parent, project string) {
		prose := filepath.Join(w.dir, "prose.md")
		_ = os.WriteFile(prose, []byte("overall plan prose"), 0o644)
		sf := filepath.Join(w.dir, "steps.json")
		_ = os.WriteFile(sf, []byte(steps), 0o644)
		args := []string{"plan", "build", "--title", title, "--prose", prose, "--steps", sf, "--json"}
		if parent != "" {
			args = append(args, "--parent", parent)
		}
		if project != "" {
			args = append(args, "--project", project)
		}
		out, err := w.runCLI(args...)
		w.err = err
		planResp = planBuildResult{}
		if err == nil {
			w.err = json.Unmarshal([]byte(strings.TrimSpace(out)), &planResp)
		}
	}

	approve := func(id string) domain.Issue {
		out, err := w.runCLI("plan", "approve", id, "--json")
		w.err = err
		var iss domain.Issue
		if err == nil {
			w.err = json.Unmarshal([]byte(strings.TrimSpace(out)), &iss)
		}
		return iss
	}

	// --- Build a plan into a ticket tree ---
	sc.Step(`^plan prose and an ordered list of 3 tracer items each with a subject and body$`, func() error {
		pendingSteps = threeSteps
		return nil
	})
	sc.Step(`^I build a plan from them$`, func() error {
		build("The Plan", pendingSteps, "", "")
		return nil
	})
	sc.Step(`^a plan issue is created with type plan, approval pending, and the prose as its body$`, func() error {
		if w.err != nil {
			return fmt.Errorf("build failed: %w", w.err)
		}
		p := planResp.Plan
		if p.Type != domain.TypePlan {
			return fmt.Errorf("type = %q, want plan", p.Type)
		}
		if p.Approval != domain.ApprovalPending {
			return fmt.Errorf("approval = %q, want pending", p.Approval)
		}
		if p.Body != "overall plan prose" {
			return fmt.Errorf("body = %q, want the prose", p.Body)
		}
		return nil
	})
	sc.Step(`^(\d+) child issues are created, one per tracer item, each with parent_id set to the plan issue$`, func(n int) error {
		if len(planResp.Children) != n {
			return fmt.Errorf("children = %d, want %d", len(planResp.Children), n)
		}
		for i, c := range planResp.Children {
			if c.ParentID == nil || *c.ParentID != planResp.Plan.ID {
				return fmt.Errorf("child %d parent = %v, want %s", i, c.ParentID, planResp.Plan.ID)
			}
		}
		return nil
	})
	sc.Step(`^the children are chained in order so tracer 1 blocks tracer 2 and tracer 2 blocks tracer 3$`, func() error {
		c := planResp.Children
		if len(c) != 3 {
			return fmt.Errorf("expected 3 children, got %d", len(c))
		}
		// Assert the children match the tracer input in order, so a reordered or
		// duplicated response can't pass just because the chain follows it.
		wantSubject := []string{"one", "two", "three"}
		wantBody := []string{"b1", "b2", "b3"}
		for i := range c {
			if c[i].Subject != wantSubject[i] || c[i].Body != wantBody[i] {
				return fmt.Errorf("child %d = %q/%q, want %q/%q (tracer run order)", i, c[i].Subject, c[i].Body, wantSubject[i], wantBody[i])
			}
		}
		v1, err := w.verify.GetIssueView(c[1].ID)
		if err != nil {
			return err
		}
		if !containsStr(v1.BlockedBy, c[0].ID) {
			return fmt.Errorf("tracer 2 blocked_by = %v, want to contain %s", v1.BlockedBy, c[0].ID)
		}
		v2, err := w.verify.GetIssueView(c[2].ID)
		if err != nil {
			return err
		}
		if !containsStr(v2.BlockedBy, c[1].ID) {
			return fmt.Errorf("tracer 3 blocked_by = %v, want to contain %s", v2.BlockedBy, c[1].ID)
		}
		return nil
	})
	sc.Step(`^a plan tree is built$`, func() error {
		if planResp.Plan.ID == "" {
			build("The Plan", threeSteps, "", "")
		}
		return w.err
	})
	sc.Step(`^a LedgerEntry of kind created is appended for the plan issue and for each child$`, func() error {
		ids := append([]string{planResp.Plan.ID}, childIDs(planResp.Children)...)
		for _, id := range ids {
			hist, err := w.verify.IssueHistory(id)
			if err != nil {
				return err
			}
			if !hasLedgerKind(hist, domain.LedgerCreated) {
				return fmt.Errorf("issue %s has no created ledger entry", id)
			}
		}
		return nil
	})
	sc.Step(`^plan prose and an ordered list of 1 tracer item$`, func() error {
		pendingSteps = oneStep
		return nil
	})
	sc.Step(`^I build a plan from it$`, func() error {
		build("Single", pendingSteps, "", "")
		return nil
	})
	sc.Step(`^a plan issue with exactly 1 child is created and no blocking edges exist$`, func() error {
		if w.err != nil {
			return fmt.Errorf("build failed: %w", w.err)
		}
		if len(planResp.Children) != 1 {
			return fmt.Errorf("children = %d, want 1", len(planResp.Children))
		}
		v, err := w.verify.GetIssueView(planResp.Children[0].ID)
		if err != nil {
			return err
		}
		if len(v.BlockedBy) != 0 || len(v.IsBlocking) != 0 {
			return fmt.Errorf("single child has blocking edges: blocked_by=%v is_blocking=%v", v.BlockedBy, v.IsBlocking)
		}
		return nil
	})
	sc.Step(`^an existing feature issue that belongs to a project$`, func() error {
		var err error
		if featProject, err = createProject("Feat proj", "/repos/featproj"); err != nil {
			return err
		}
		out, err := w.runCLI("create", "--subject", "feature", "--body", "b", "--project", featProject, "--json")
		if err != nil {
			return err
		}
		var feat domain.Issue
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &feat); err != nil {
			return err
		}
		featureID = feat.ID
		// Make it a genuine feature issue (create defaults to task) and confirm,
		// so this fixture actually exercises the feature-parent path.
		upd, err := w.updateIssue(featureID, "--type", "feature")
		if err != nil {
			return err
		}
		if upd.Type != domain.TypeFeature {
			return fmt.Errorf("fixture type = %q, want feature", upd.Type)
		}
		return nil
	})
	sc.Step(`^I build a plan referencing that feature issue as parent$`, func() error {
		build("Under feature", twoSteps, featureID, "")
		return nil
	})
	sc.Step(`^the plan issue's parent_id is the feature issue$`, func() error {
		if w.err != nil {
			return fmt.Errorf("build failed: %w", w.err)
		}
		if planResp.Plan.ParentID == nil || *planResp.Plan.ParentID != featureID {
			return fmt.Errorf("plan parent = %v, want %s", planResp.Plan.ParentID, featureID)
		}
		return nil
	})
	sc.Step(`^the plan issue and its children carry the feature issue's project_id$`, func() error {
		return allCarryProject(planResp, featProject)
	})
	sc.Step(`^plan prose, tracer items, and an explicit project_id$`, func() error {
		var err error
		explicitProject, err = createProject("Explicit", "/repos/explicit")
		pendingSteps = twoSteps
		return err
	})
	sc.Step(`^I build a plan with that project_id and no parent$`, func() error {
		build("Explicit project", pendingSteps, "", explicitProject)
		return nil
	})
	sc.Step(`^the plan issue and its children carry that project_id$`, func() error {
		return allCarryProject(planResp, explicitProject)
	})
	sc.Step(`^plan prose and tracer items with neither a parent nor a project_id$`, func() error {
		pendingSteps = twoSteps
		return nil
	})
	sc.Step(`^the plan issue and its children have no project_id$`, func() error {
		if w.err != nil {
			return fmt.Errorf("build failed: %w", w.err)
		}
		if planResp.Plan.ProjectID != nil {
			return fmt.Errorf("plan project = %v, want none", *planResp.Plan.ProjectID)
		}
		for i, c := range planResp.Children {
			if c.ProjectID != nil {
				return fmt.Errorf("child %d project = %v, want none", i, *c.ProjectID)
			}
		}
		return nil
	})

	// --- Building a plan rejects invalid tracer input ---
	sc.Step(`^plan prose and tracer items where one has an empty subject$`, func() error {
		pendingSteps = `[{"subject":"ok","body":"b"},{"subject":"","body":"b"}]`
		return nil
	})
	sc.Step(`^plan prose and tracer items where one has type epic$`, func() error {
		pendingSteps = `[{"subject":"ok","body":"b","type":"epic"}]`
		return nil
	})
	sc.Step(`^plan prose and an empty list of tracer items$`, func() error {
		pendingSteps = `[]`
		return nil
	})
	sc.Step(`^plan prose and tracer items where one has type plan$`, func() error {
		pendingSteps = `[{"subject":"ok","body":"b"},{"subject":"nested","body":"b","type":"plan"}]`
		return nil
	})
	sc.Step(`^I try to build the plan$`, func() error {
		build("Bad plan", pendingSteps, "", "")
		return nil
	})
	sc.Step(`^no plan issue and no child issues are created$`, func() error {
		// Assert the entire live issue set is empty — this catches a leaked plan
		// issue AND any leaked (non-plan) tracer children.
		all, err := w.verify.ListIssues(domain.IssueFilter{})
		if err != nil {
			return err
		}
		if len(all) != 0 {
			return fmt.Errorf("expected no issues created, found %d", len(all))
		}
		return nil
	})

	// --- Approve a pending plan ---
	sc.Step(`^a plan issue with approval pending$`, func() error {
		build("Approvable", twoSteps, "", "")
		return w.err
	})
	sc.Step(`^I approve the plan issue$`, func() error {
		approved = approve(planResp.Plan.ID)
		return nil
	})
	sc.Step(`^its approval becomes approved and updated_at advances$`, func() error {
		if w.err != nil {
			return fmt.Errorf("approve failed: %w", w.err)
		}
		if approved.Approval != domain.ApprovalApproved {
			return fmt.Errorf("approval = %q, want approved", approved.Approval)
		}
		if !approved.UpdatedAt.After(planResp.Plan.UpdatedAt) {
			return fmt.Errorf("updated_at did not advance: was %s, now %s", planResp.Plan.UpdatedAt, approved.UpdatedAt)
		}
		return nil
	})
	sc.Step(`^an approval is recorded$`, func() error { return nil })
	sc.Step(`^a LedgerEntry of kind updated with field approval and new_value approved is appended$`, func() error {
		hist, err := w.verify.IssueHistory(planResp.Plan.ID)
		if err != nil {
			return err
		}
		for _, e := range hist {
			if e.Kind == domain.LedgerUpdated && e.Field == "approval" && e.NewValue == string(domain.ApprovalApproved) {
				return nil
			}
		}
		return fmt.Errorf("no updated/approval/approved ledger entry in %d entries", len(hist))
	})
	sc.Step(`^a plan issue that is already approved$`, func() error {
		build("Twice", twoSteps, "", "")
		if w.err != nil {
			return w.err
		}
		approved = approve(planResp.Plan.ID)
		if w.err != nil {
			return w.err
		}
		hist, err := w.verify.IssueHistory(planResp.Plan.ID)
		if err != nil {
			return err
		}
		ledgerBefore = len(hist)
		return nil
	})
	sc.Step(`^I approve it again$`, func() error {
		approved = approve(planResp.Plan.ID)
		return nil
	})
	sc.Step(`^its approval is still approved and no additional LedgerEntry is appended$`, func() error {
		if w.err != nil {
			return fmt.Errorf("re-approve failed: %w", w.err)
		}
		if approved.Approval != domain.ApprovalApproved {
			return fmt.Errorf("approval = %q, want approved", approved.Approval)
		}
		hist, err := w.verify.IssueHistory(planResp.Plan.ID)
		if err != nil {
			return err
		}
		if len(hist) != ledgerBefore {
			return fmt.Errorf("ledger grew on idempotent approve: %d -> %d", ledgerBefore, len(hist))
		}
		return nil
	})

	// --- Approval rejects invalid targets ---
	sc.Step(`^an issue of type task$`, func() error {
		t, err := w.create("a task", "b")
		taskID = t.ID
		return err
	})
	sc.Step(`^I try to approve it as a plan$`, func() error {
		approve(taskID)
		return nil
	})
	sc.Step(`^it is rejected and the issue is unchanged$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected rejection, got none")
		}
		iss, err := w.verify.GetIssue(taskID)
		if err != nil {
			return err
		}
		if iss.Approval != "" {
			return fmt.Errorf("task approval = %q, want unchanged empty", iss.Approval)
		}
		return nil
	})
	sc.Step(`^I try to change its type to plan$`, func() error {
		_, w.err = w.updateIssue(taskID, "--type", "plan")
		return nil
	})
	sc.Step(`^no issue exists with the given id$`, func() error { return nil })
	sc.Step(`^I try to approve that id as a plan$`, func() error {
		approve("no-such-id")
		return nil
	})
	sc.Step(`^it is rejected as not found$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected not-found rejection, got none")
		}
		if !strings.Contains(w.err.Error(), "404") {
			return fmt.Errorf("error %q does not indicate 404 not found", w.err.Error())
		}
		return nil
	})
}

func childIDs(children []domain.Issue) []string {
	ids := make([]string, len(children))
	for i, c := range children {
		ids[i] = c.ID
	}
	return ids
}

func hasLedgerKind(entries []domain.LedgerEntry, kind domain.LedgerKind) bool {
	for _, e := range entries {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func allCarryProject(res planBuildResult, projectID string) error {
	if res.Plan.ProjectID == nil || *res.Plan.ProjectID != projectID {
		return fmt.Errorf("plan project = %v, want %s", res.Plan.ProjectID, projectID)
	}
	for i, c := range res.Children {
		if c.ProjectID == nil || *c.ProjectID != projectID {
			return fmt.Errorf("child %d project = %v, want %s", i, c.ProjectID, projectID)
		}
	}
	return nil
}
