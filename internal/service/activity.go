package service

import (
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// Activity returns the reverse-chronological feed of issue changes and captured
// transcripts at or after `since`. It first auto-ingests any Claude session on
// disk modified within the window, so recent work shows up even when it was
// never ingested manually.
func (s *Service) Activity(since time.Time) (domain.ActivityFeed, error) {
	if err := s.ingestRecentSessions(since); err != nil {
		return domain.ActivityFeed{}, err
	}
	events, err := s.store.ActivitySince(since)
	if err != nil {
		return domain.ActivityFeed{}, err
	}
	if events == nil {
		events = []domain.ActivityEvent{}
	}
	return domain.ActivityFeed{Since: since, Events: events}, nil
}

// ingestRecentSessions ingests each discovered Claude session whose file was
// modified within the window and has actually changed since its last ingest.
// Skipping unchanged sessions is essential: re-ingesting a linked transcript
// writes a ledger entry, so merely viewing activity must not manufacture new
// activity.
func (s *Service) ingestRecentSessions(since time.Time) error {
	discovered, err := s.DiscoverTranscripts("")
	if err != nil {
		return err
	}
	ingested, err := s.store.IngestedSourceMtimes()
	if err != nil {
		return err
	}
	for _, d := range discovered {
		if d.ModTime.Before(since) {
			continue // outside the window
		}
		if prev, ok := ingested[d.SessionID]; ok && !d.ModTime.After(prev) {
			continue // already captured and unchanged since
		}
		if _, err := s.IngestTranscript(d.Path); err != nil {
			return fmt.Errorf("auto-ingest %s: %w", d.Path, err)
		}
	}
	return nil
}
