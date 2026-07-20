package service_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// noTimestampLines is a session whose events carry no timestamp, so its
// captured_at falls back to the file mtime — lets a test control the window
// deterministically via os.Chtimes.
func noTimestampLines() []string {
	return []string{
		`{"type":"user","message":{"role":"user","content":"Investigate the flaky test"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Looking now."}]}}`,
	}
}

func TestActivityAutoIngestsRecentSessionAndBuildsFeed(t *testing.T) {
	projects := t.TempDir()
	svc := newService(t, projects)

	// An issue change produces ledger events (at least "created").
	iss, err := svc.CreateIssue("Wire up search", "FTS across issues")
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	// A recent, not-yet-ingested session on disk.
	path := writeSession(t, filepath.Join(projects, "-proj"), "sess-recent", noTimestampLines())
	recent := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, recent, recent); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	feed, err := svc.Activity(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}

	var sawTranscript, sawIssue bool
	for _, e := range feed.Events {
		switch e.Type {
		case domain.ActivityTranscript:
			if e.Title == "Investigate the flaky test" {
				sawTranscript = true
			}
		case domain.ActivityLedger:
			if e.IssueID == iss.ID && e.LedgerKind == domain.LedgerCreated {
				sawIssue = true
			}
		}
	}
	if !sawTranscript {
		t.Error("feed missing the auto-ingested transcript")
	}
	if !sawIssue {
		t.Error("feed missing the issue's created ledger event")
	}

	// Newest-first ordering.
	for i := 1; i < len(feed.Events); i++ {
		if feed.Events[i-1].At.Before(feed.Events[i].At) {
			t.Errorf("feed not newest-first at %d: %v before %v", i, feed.Events[i-1].At, feed.Events[i].At)
		}
	}
}

func TestActivityWindowExcludesOlder(t *testing.T) {
	projects := t.TempDir()
	svc := newService(t, projects)

	// A session whose file (and thus captured_at) is well outside the window.
	path := writeSession(t, filepath.Join(projects, "-proj"), "sess-old", noTimestampLines())
	old := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	feed, err := svc.Activity(time.Now().Add(-72 * time.Hour))
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	for _, e := range feed.Events {
		if e.Type == domain.ActivityTranscript && e.TranscriptID != "" && e.Title == "Investigate the flaky test" {
			t.Errorf("old session should be excluded from the window, got %+v", e)
		}
	}
}
