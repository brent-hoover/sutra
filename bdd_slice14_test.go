package main_test

// Step definitions for the @slice14 scenarios: enumerating a plan's tracer
// children via the parent filter (in run order, with status) and seeing a
// completed tracer reflected on the next traversal. Each step drives the real
// `sutra` CLI against the live daemon.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/domain"
)

func registerSlice14Steps(sc *godog.ScenarioContext, w *world) {
	var (
		planID       string
		children     []domain.Issue
		leafID       string
		statusFilter string
		listOut      []domain.Issue
	)

	buildApproved := func(stepsJSON string) error {
		prose := filepath.Join(w.dir, "t14-prose.md")
		_ = os.WriteFile(prose, []byte("prose"), 0o644)
		sf := filepath.Join(w.dir, "t14-steps.json")
		_ = os.WriteFile(sf, []byte(stepsJSON), 0o644)
		out, err := w.runCLI("plan", "build", "--title", "Plan", "--prose", prose, "--steps", sf, "--json")
		if err != nil {
			return err
		}
		var res planBuildResult
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
			return err
		}
		planID = res.Plan.ID
		children = res.Children
		_, err = w.runCLI("plan", "approve", planID, "--json")
		return err
	}

	listParent := func(parent string, extra ...string) {
		args := append([]string{"list", "--parent", parent}, extra...)
		args = append(args, "--json")
		out, err := w.runCLI(args...)
		w.err = err
		listOut = nil
		if err == nil {
			w.err = json.Unmarshal([]byte(strings.TrimSpace(out)), &listOut)
		}
	}

	find := func(id string) (domain.Issue, bool) {
		for _, i := range listOut {
			if i.ID == id {
				return i, true
			}
		}
		return domain.Issue{}, false
	}

	// --- Enumerate a plan's tracers in order ---
	sc.Step(`^an approved plan issue with 3 tracer children chained in order, all open$`, func() error {
		return buildApproved(`[{"subject":"one"},{"subject":"two"},{"subject":"three"}]`)
	})
	sc.Step(`^I list issues filtered by the plan issue as parent$`, func() error {
		listParent(planID)
		return w.err
	})
	sc.Step(`^exactly the 3 tracer children are returned in run order, each with its status$`, func() error {
		if len(listOut) != 3 {
			return fmt.Errorf("got %d children, want 3", len(listOut))
		}
		for i, c := range children {
			if listOut[i].ID != c.ID {
				return fmt.Errorf("position %d = %s, want %s (run order)", i, listOut[i].ID, c.ID)
			}
			if listOut[i].Status == "" {
				return fmt.Errorf("child %s has no status", listOut[i].ID)
			}
		}
		return nil
	})
	sc.Step(`^an issue with no children$`, func() error {
		iss, err := w.create("leaf", "no children")
		leafID = iss.ID
		return err
	})
	sc.Step(`^I list issues filtered by that issue as parent$`, func() error {
		listParent(leafID)
		return w.err
	})
	sc.Step(`^no issues are returned$`, func() error {
		if len(listOut) != 0 {
			return fmt.Errorf("expected no issues, got %d", len(listOut))
		}
		return nil
	})
	sc.Step(`^a status filter of done$`, func() error {
		statusFilter = "done"
		return nil
	})
	sc.Step(`^I list a plan's children with that filter$`, func() error {
		listParent(planID, "--status", statusFilter)
		return nil
	})
	sc.Step(`^it is rejected as an invalid filter$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected an invalid-filter rejection, got none")
		}
		return nil
	})

	// --- Completing a tracer is reflected on the next traversal ---
	sc.Step(`^an approved plan whose first tracer is open and blocks the second$`, func() error {
		return buildApproved(`[{"subject":"one"},{"subject":"two"}]`)
	})
	sc.Step(`^I close the first tracer and list the plan's children again$`, func() error {
		if _, err := w.updateIssue(children[0].ID, "--status", "closed"); err != nil {
			return err
		}
		listParent(planID)
		return w.err
	})
	sc.Step(`^the first tracer is returned with status closed$`, func() error {
		c0, ok := find(children[0].ID)
		if !ok {
			return fmt.Errorf("first tracer missing from list")
		}
		if c0.Status != domain.StatusClosed {
			return fmt.Errorf("first tracer status = %q, want closed", c0.Status)
		}
		return nil
	})
	sc.Step(`^the second tracer is returned with status open$`, func() error {
		c1, ok := find(children[1].ID)
		if !ok {
			return fmt.Errorf("second tracer missing from list")
		}
		if c1.Status != domain.StatusOpen {
			return fmt.Errorf("second tracer status = %q, want open", c1.Status)
		}
		return nil
	})
	sc.Step(`^the block edge from the first tracer to the second still exists$`, func() error {
		v, err := w.verify.GetIssueView(children[1].ID)
		if err != nil {
			return err
		}
		if !containsStr(v.BlockedBy, children[0].ID) {
			return fmt.Errorf("second tracer blocked_by = %v, want to still contain %s", v.BlockedBy, children[0].ID)
		}
		return nil
	})
}
