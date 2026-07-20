package main_test

// Step definitions for the @slice10 project scenarios: project CRUD, scoping an
// issue to a project, and transcripts auto-associating to a project by matching
// their session working directory to the project's repo path. Steps drive the
// real CLI against the running daemon.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/domain"
)

func registerSlice10Steps(sc *godog.ScenarioContext, w *world) {
	var (
		repoPath   string
		projectID  string
		projSlug   string
		issueID    string
		listOut    string
		updatedOut string
		transProj  string
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
		projSlug = p.Slug
		return p.ID, nil
	}

	// --- Create a project ---
	sc.Step(`^a name and a repo path$`, func() error {
		repoPath = "/Users/tester/Projects/alpha"
		return nil
	})
	sc.Step(`^I create a project$`, func() error {
		var err error
		projectID, err = createProject("Alpha", repoPath)
		return err
	})
	sc.Step(`^it is stored with a generated id, a slug, and timestamps set$`, func() error {
		if projectID == "" {
			return fmt.Errorf("expected a generated id")
		}
		if projSlug != "alpha" {
			return fmt.Errorf("slug = %q, want %q", projSlug, "alpha")
		}
		return nil
	})

	// --- Repo path is unique ---
	sc.Step(`^a project already exists for a repo path$`, func() error {
		repoPath = "/Users/tester/Projects/dup"
		var err error
		projectID, err = createProject("Dup", repoPath)
		return err
	})
	sc.Step(`^I create another project for the same repo path$`, func() error {
		// The shared "^it is rejected$" step asserts on w.err.
		_, w.err = w.runCLI("project", "create", "--name", "Dup2", "--repo", repoPath, "--json")
		return nil
	})

	// --- List, view, update ---
	sc.Step(`^some projects exist$`, func() error {
		if _, err := createProject("One", "/Users/tester/Projects/one"); err != nil {
			return err
		}
		var err error
		projectID, err = createProject("Two", "/Users/tester/Projects/two")
		return err
	})
	sc.Step(`^I list them, view one, and update its name$`, func() error {
		out, err := w.runCLI("project", "list", "--json")
		if err != nil {
			return err
		}
		listOut = out
		if _, err := w.runCLI("project", "view", projectID, "--json"); err != nil {
			return err
		}
		updatedOut, err = w.runCLI("project", "update", projectID, "--name", "Two Renamed", "--json")
		return err
	})
	sc.Step(`^the changes are reflected$`, func() error {
		var projects []domain.Project
		if err := json.Unmarshal([]byte(strings.TrimSpace(listOut)), &projects); err != nil {
			return err
		}
		if len(projects) < 2 {
			return fmt.Errorf("expected at least 2 projects, got %d", len(projects))
		}
		var p domain.Project
		if err := json.Unmarshal([]byte(strings.TrimSpace(updatedOut)), &p); err != nil {
			return err
		}
		if p.Name != "Two Renamed" {
			return fmt.Errorf("updated name = %q, want %q", p.Name, "Two Renamed")
		}
		return nil
	})

	// --- Delete detaches references ---
	sc.Step(`^a project with an issue scoped to it$`, func() error {
		var err error
		projectID, err = createProject("Scoped", "/Users/tester/Projects/scoped")
		if err != nil {
			return err
		}
		out, err := w.runCLI("create", "--subject", "scoped issue", "--body", "b", "--project", projectID, "--json")
		if err != nil {
			return err
		}
		var iss domain.Issue
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &iss); err != nil {
			return err
		}
		issueID = iss.ID
		if iss.ProjectID == nil || *iss.ProjectID != projectID {
			return fmt.Errorf("issue not scoped to project: %v", iss.ProjectID)
		}
		return nil
	})
	sc.Step(`^I delete the project$`, func() error {
		_, err := w.runCLI("project", "delete", projectID)
		return err
	})
	sc.Step(`^the project is gone and the issue's project is cleared$`, func() error {
		if _, err := w.runCLI("project", "view", projectID, "--json"); err == nil {
			return fmt.Errorf("expected project to be gone")
		}
		out, err := w.runCLI("view", issueID, "--json")
		if err != nil {
			return err
		}
		var iss domain.Issue
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &iss); err != nil {
			return err
		}
		if iss.ProjectID != nil {
			return fmt.Errorf("issue project not cleared: %v", *iss.ProjectID)
		}
		return nil
	})

	// --- Scope an issue to a project + filter ---
	sc.Step(`^a project$`, func() error {
		var err error
		projectID, err = createProject("Filterable", "/Users/tester/Projects/filterable")
		return err
	})
	sc.Step(`^I create an issue in that project$`, func() error {
		out, err := w.runCLI("create", "--subject", "in project", "--body", "b", "--project", projectID, "--json")
		if err != nil {
			return err
		}
		var iss domain.Issue
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &iss); err != nil {
			return err
		}
		issueID = iss.ID
		return nil
	})
	sc.Step(`^the issue lists that project and appears when filtering by it$`, func() error {
		out, err := w.runCLI("list", "--project", projectID, "--json")
		if err != nil {
			return err
		}
		var issues []domain.Issue
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &issues); err != nil {
			return err
		}
		for _, i := range issues {
			if i.ID == issueID {
				return nil
			}
		}
		return fmt.Errorf("issue %s not found when filtering by project %s", issueID, projectID)
	})

	// --- Transcript auto-association by repo path ---
	sc.Step(`^a project whose repo path matches a session's working directory$`, func() error {
		repoPath = "/Users/tester/Projects/autolink"
		var err error
		projectID, err = createProject("Autolink", repoPath)
		if err != nil {
			return err
		}
		// Write a session under the encoded-cwd folder matching the repo path.
		dir := filepath.Join(w.dir, domain.EncodeCWD(repoPath))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(
			filepath.Join(dir, "auto0000-0000-0000-0000-000000000010.jsonl"),
			[]byte(`{"type":"user","message":{"role":"user","content":"hello"}}`+"\n"), 0o644)
	})
	sc.Step(`^the session transcript is ingested$`, func() error {
		path := filepath.Join(w.dir, domain.EncodeCWD(repoPath), "auto0000-0000-0000-0000-000000000010.jsonl")
		out, err := w.runCLI("transcript", "ingest", path, "--json")
		if err != nil {
			return err
		}
		var tr domain.Transcript
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &tr); err != nil {
			return err
		}
		if tr.ProjectID != nil {
			transProj = *tr.ProjectID
		}
		return nil
	})
	sc.Step(`^the transcript is associated with that project$`, func() error {
		if transProj != projectID {
			return fmt.Errorf("transcript project = %q, want %q", transProj, projectID)
		}
		return nil
	})
}
