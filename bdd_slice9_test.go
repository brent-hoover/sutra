package main_test

// Step definitions for the @slice9 activity-feed scenarios: a
// reverse-chronological feed across issue changes and captured transcripts,
// scoped to a time window, that auto-ingests recent sessions first. Steps drive
// the real CLI against the running daemon; sessions are written under the
// daemon's projects dir (w.dir) with controlled mtimes.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// noTSSessionLines is a Claude session whose events carry no timestamp, so the
// ingested captured_at falls back to the file mtime — letting a scenario place
// the session inside or outside the window deterministically via os.Chtimes.
func noTSSessionLines(firstUserMessage string) []string {
	return []string{
		fmt.Sprintf(`{"type":"user","message":{"role":"user","content":%q}}`, firstUserMessage),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"On it."}]}}`,
	}
}

func registerSlice9Steps(sc *godog.ScenarioContext, w *world) {
	var (
		issueID     string
		recentTtl   string // title of a session placed within the window
		oldTitle    string // title of a session placed outside the window
		activityout string // captured stdout of the activity command
	)

	// writeSessionAt writes a session under the daemon's projects dir and stamps
	// its mtime, returning the derived transcript title (its first user message).
	writeSessionAt := func(sessionID, firstUserMessage string, mtime time.Time) error {
		path, err := writeSession(filepath.Join(w.dir, "-proj"), sessionID, noTSSessionLines(firstUserMessage))
		if err != nil {
			return err
		}
		return os.Chtimes(path, mtime, mtime)
	}

	createAndChangeIssue := func(subject string) error {
		out, err := w.runCLI("create", "--subject", subject, "--body", "b", "--json")
		if err != nil {
			return err
		}
		var iss struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &iss); err != nil {
			return err
		}
		issueID = iss.ID
		_, err = w.runCLI("update", issueID, "--status", "in_progress")
		return err
	}

	// --- Scenario: Catch up on recent activity ---
	sc.Step(`^issues that changed and a transcript captured within the window$`, func() error {
		if err := createAndChangeIssue("Wire up search"); err != nil {
			return err
		}
		recentTtl = "Investigate the flaky login test"
		// Not ingested: activity must auto-ingest it.
		return writeSessionAt("sess-recent-0000-0000-0000-000000000001", recentTtl, time.Now().Add(-1*time.Hour))
	})
	sc.Step(`^I see a reverse-chronological feed of the issue changes and the transcript$`, func() error {
		if w.err != nil {
			return w.err
		}
		if !strings.Contains(activityout, issueID) {
			return fmt.Errorf("feed missing issue %s:\n%s", issueID, activityout)
		}
		if !strings.Contains(activityout, recentTtl) {
			return fmt.Errorf("feed missing transcript %q:\n%s", recentTtl, activityout)
		}
		return nil
	})

	// --- Scenario: Auto-ingest surfaces an un-captured session ---
	sc.Step(`^a Claude session file on disk within the window that has not been ingested$`, func() error {
		recentTtl = "Draft the migration plan"
		return writeSessionAt("sess-uncaptured-0000-0000-0000-000000000002", recentTtl, time.Now().Add(-2*time.Hour))
	})
	sc.Step(`^the session is ingested and appears in the feed$`, func() error {
		if w.err != nil {
			return w.err
		}
		// The feed reads the DB, so the title can only appear if activity ingested
		// the on-disk session.
		if !strings.Contains(activityout, recentTtl) {
			return fmt.Errorf("auto-ingested session %q not in feed:\n%s", recentTtl, activityout)
		}
		// Confirm it is now recorded as ingested.
		disc, err := w.runCLI("transcript", "discover", "--json")
		if err != nil {
			return err
		}
		var found []struct {
			SessionID string `json:"session_id"`
			Ingested  bool   `json:"ingested"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(disc)), &found); err != nil {
			return err
		}
		for _, d := range found {
			if strings.HasPrefix(d.SessionID, "sess-uncaptured") {
				if !d.Ingested {
					return fmt.Errorf("session %s still marked not ingested", d.SessionID)
				}
				return nil
			}
		}
		return fmt.Errorf("uncaptured session not found in discover output: %s", disc)
	})

	// --- Scenario: The window excludes older activity ---
	sc.Step(`^activity older than the window and activity within it$`, func() error {
		if err := createAndChangeIssue("Recent work"); err != nil {
			return err
		}
		oldTitle = "Ancient exploration from last month"
		if err := writeSessionAt("sess-old-0000-0000-0000-000000000003", oldTitle, time.Now().Add(-30*24*time.Hour)); err != nil {
			return err
		}
		// Ingest the old session now so it is genuinely in the DB with an old
		// captured_at — the feed's window filter (not just the auto-ingest gate)
		// must exclude it.
		path := filepath.Join(w.dir, "-proj", "sess-old-0000-0000-0000-000000000003.jsonl")
		_, err := w.runCLI("transcript", "ingest", path)
		return err
	})
	sc.Step(`^only the activity within the window is shown$`, func() error {
		if w.err != nil {
			return w.err
		}
		if !strings.Contains(activityout, issueID) {
			return fmt.Errorf("feed missing recent issue %s:\n%s", issueID, activityout)
		}
		if strings.Contains(activityout, oldTitle) {
			return fmt.Errorf("feed should exclude old activity %q:\n%s", oldTitle, activityout)
		}
		return nil
	})

	// Shared When: run the activity command over a 24h window.
	sc.Step(`^I run sutra activity for that window$`, func() error {
		activityout, w.err = w.runCLI("activity", "--since", "24h")
		return nil
	})
}
