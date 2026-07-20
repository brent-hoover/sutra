package main_test

// Step definitions for the @slice11 thread scenarios: creating threads, attaching
// heterogeneous members (issues, documents, transcripts, comments), viewing them
// grouped, many-to-many membership, detach, delete, and project scoping. Steps
// drive the real CLI against the running daemon.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/domain"
)

func registerSlice11Steps(sc *godog.ScenarioContext, w *world) {
	var (
		threadID   string
		threadB    string
		created    domain.Thread
		issueID    string
		docID      string
		transID    string
		commentID  string
		projectID  string
		threadView domain.ThreadView
	)

	createThread := func(title string, extra ...string) (domain.Thread, error) {
		args := append([]string{"thread", "create", "--title", title, "--json"}, extra...)
		out, err := w.runCLI(args...)
		if err != nil {
			return domain.Thread{}, err
		}
		var t domain.Thread
		err = json.Unmarshal([]byte(strings.TrimSpace(out)), &t)
		return t, err
	}
	viewThread := func(id string) (domain.ThreadView, error) {
		out, err := w.runCLI("thread", "view", id, "--json")
		if err != nil {
			return domain.ThreadView{}, err
		}
		var v domain.ThreadView
		err = json.Unmarshal([]byte(strings.TrimSpace(out)), &v)
		return v, err
	}
	idFrom := func(out string) (string, error) {
		var v struct {
			ID string `json:"id"`
		}
		err := json.Unmarshal([]byte(strings.TrimSpace(out)), &v)
		return v.ID, err
	}

	// --- Create a thread ---
	sc.Step(`^a title$`, func() error { return nil })
	sc.Step(`^I create a thread$`, func() error {
		var err error
		created, err = createThread("My line of work")
		threadID = created.ID
		return err
	})
	sc.Step(`^it is stored with a generated id, status active, and timestamps set$`, func() error {
		if created.ID == "" {
			return fmt.Errorf("expected a generated id")
		}
		if created.Status != domain.ThreadActive {
			return fmt.Errorf("status = %q, want active", created.Status)
		}
		if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
			return fmt.Errorf("expected timestamps set")
		}
		return nil
	})

	// --- Attach items of every kind and view ---
	sc.Step(`^a thread and an issue, a document, a transcript, and a comment$`, func() error {
		t, err := createThread("Everything")
		if err != nil {
			return err
		}
		threadID = t.ID

		issOut, err := w.runCLI("create", "--subject", "an issue", "--body", "b", "--json")
		if err != nil {
			return err
		}
		if issueID, err = idFrom(issOut); err != nil {
			return err
		}
		docOut, err := w.runCLI("doc", "attach", issueID, "--kind", "problem", "--title", "P", "--content", "c", "--json")
		if err != nil {
			return err
		}
		if docID, err = idFrom(docOut); err != nil {
			return err
		}
		comOut, err := w.runCLI("comment", issueID, "--body", "a comment", "--json")
		if err != nil {
			return err
		}
		if commentID, err = idFrom(comOut); err != nil {
			return err
		}
		// A transcript: write a session under the projects dir and ingest it.
		dir := filepath.Join(w.dir, "-thread-proj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, "thrd0000-0000-0000-0000-000000000011.jsonl")
		if err := os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0o644); err != nil {
			return err
		}
		trOut, err := w.runCLI("transcript", "ingest", path, "--json")
		if err != nil {
			return err
		}
		transID, err = idFrom(trOut)
		return err
	})
	sc.Step(`^I attach each item to the thread$`, func() error {
		for _, pair := range [][2]string{
			{"issue", issueID}, {"document", docID}, {"transcript", transID}, {"comment", commentID},
		} {
			if _, err := w.runCLI("thread", "add", threadID, pair[0], pair[1]); err != nil {
				return fmt.Errorf("attach %s: %w", pair[0], err)
			}
		}
		return nil
	})
	sc.Step(`^viewing the thread shows all four grouped by kind$`, func() error {
		v, err := viewThread(threadID)
		if err != nil {
			return err
		}
		want := map[domain.ThreadItemKind]string{
			domain.ThreadItemIssue:      issueID,
			domain.ThreadItemDocument:   docID,
			domain.ThreadItemTranscript: transID,
			domain.ThreadItemComment:    commentID,
		}
		found := map[domain.ThreadItemKind]bool{}
		for _, it := range v.Items {
			if want[it.Kind] == it.ItemID {
				found[it.Kind] = true
			}
		}
		for kind := range want {
			if !found[kind] {
				return fmt.Errorf("thread view missing %s member", kind)
			}
		}
		return nil
	})

	// --- Attaching a nonexistent item is rejected ---
	sc.Step(`^a thread$`, func() error {
		t, err := createThread("Bare")
		threadID = t.ID
		return err
	})
	sc.Step(`^I attach an item id that does not exist$`, func() error {
		_, err := w.runCLI("thread", "add", threadID, "issue", "does-not-exist")
		w.err = err
		return nil
	})
	sc.Step(`^the attach is rejected$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected attaching a nonexistent item to be rejected")
		}
		return nil
	})

	// --- Many-to-many ---
	sc.Step(`^two threads and one issue$`, func() error {
		t1, err := createThread("T1")
		if err != nil {
			return err
		}
		t2, err := createThread("T2")
		if err != nil {
			return err
		}
		threadID, threadB = t1.ID, t2.ID
		out, err := w.runCLI("create", "--subject", "shared", "--body", "b", "--json")
		if err != nil {
			return err
		}
		issueID, err = idFrom(out)
		return err
	})
	sc.Step(`^I attach the issue to both threads$`, func() error {
		if _, err := w.runCLI("thread", "add", threadID, "issue", issueID); err != nil {
			return err
		}
		_, err := w.runCLI("thread", "add", threadB, "issue", issueID)
		return err
	})
	sc.Step(`^the issue appears in both$`, func() error {
		for _, id := range []string{threadID, threadB} {
			v, err := viewThread(id)
			if err != nil {
				return err
			}
			ok := false
			for _, it := range v.Items {
				if it.Kind == domain.ThreadItemIssue && it.ItemID == issueID {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("issue %s not in thread %s", issueID, id)
			}
		}
		return nil
	})

	// --- Detach ---
	sc.Step(`^a thread with an attached issue$`, func() error {
		t, err := createThread("Detachable")
		if err != nil {
			return err
		}
		threadID = t.ID
		out, err := w.runCLI("create", "--subject", "attached", "--body", "b", "--json")
		if err != nil {
			return err
		}
		if issueID, err = idFrom(out); err != nil {
			return err
		}
		_, err = w.runCLI("thread", "add", threadID, "issue", issueID)
		return err
	})
	sc.Step(`^I remove the issue from the thread$`, func() error {
		_, err := w.runCLI("thread", "remove", threadID, "issue", issueID)
		return err
	})
	sc.Step(`^the thread no longer lists it and the issue still exists$`, func() error {
		v, err := viewThread(threadID)
		if err != nil {
			return err
		}
		for _, it := range v.Items {
			if it.Kind == domain.ThreadItemIssue && it.ItemID == issueID {
				return fmt.Errorf("issue still listed after removal")
			}
		}
		if _, err := w.runCLI("view", issueID, "--json"); err != nil {
			return fmt.Errorf("issue should still exist: %w", err)
		}
		return nil
	})

	// --- Delete thread leaves members ---
	sc.Step(`^I delete the thread$`, func() error {
		_, err := w.runCLI("thread", "delete", threadID)
		return err
	})
	sc.Step(`^the thread is gone and the issue still exists$`, func() error {
		if _, err := w.runCLI("thread", "view", threadID, "--json"); err == nil {
			return fmt.Errorf("expected thread to be gone")
		}
		if _, err := w.runCLI("view", issueID, "--json"); err != nil {
			return fmt.Errorf("issue should still exist: %w", err)
		}
		return nil
	})

	// --- Scope a thread to a project ---
	sc.Step(`^a project for the thread$`, func() error {
		out, err := w.runCLI("project", "create", "--name", "ThreadProj", "--repo", "/Users/tester/Projects/threadproj", "--json")
		if err != nil {
			return err
		}
		projectID, err = idFrom(out)
		return err
	})
	sc.Step(`^I create a thread in that project$`, func() error {
		t, err := createThread("Scoped thread", "--project", projectID)
		threadID = t.ID
		threadView = domain.ThreadView{Thread: t}
		return err
	})
	sc.Step(`^the thread lists that project and appears when filtering by it$`, func() error {
		if threadView.ProjectID == nil || *threadView.ProjectID != projectID {
			return fmt.Errorf("thread not scoped to project: %v", threadView.ProjectID)
		}
		out, err := w.runCLI("thread", "list", "--project", projectID, "--json")
		if err != nil {
			return err
		}
		var threads []domain.Thread
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &threads); err != nil {
			return err
		}
		for _, t := range threads {
			if t.ID == threadID {
				return nil
			}
		}
		return fmt.Errorf("thread %s not found filtering by project %s", threadID, projectID)
	})
}
