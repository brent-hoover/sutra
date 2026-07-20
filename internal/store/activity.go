package store

import (
	"fmt"
	"sort"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// ActivitySince returns the activity feed at or after `since`: every ledger
// entry (an issue change) and every captured transcript, newest first.
//
// Timestamps are stored as RFC3339Nano text, whose trailing zeros are trimmed,
// so lexicographic (SQL) comparison is not chronologically reliable. We
// therefore parse and filter/sort in Go. For a single-user store this is cheap;
// if history ever grows large, add an indexed epoch column.
func (s *Store) ActivitySince(since time.Time) ([]domain.ActivityEvent, error) {
	events, err := s.ledgerEvents()
	if err != nil {
		return nil, err
	}
	tx, err := s.transcriptEvents()
	if err != nil {
		return nil, err
	}
	events = append(events, tx...)

	kept := events[:0]
	for _, e := range events {
		if !e.At.Before(since) {
			kept = append(kept, e)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].At.After(kept[j].At) })
	return kept, nil
}

func (s *Store) ledgerEvents() ([]domain.ActivityEvent, error) {
	rows, err := s.db.Query(
		`SELECT l.at, l.issue_id, l.kind, l.field, l.old_value, l.new_value, i.subject
		 FROM ledger l JOIN issues i ON i.id = l.issue_id`)
	if err != nil {
		return nil, fmt.Errorf("query ledger activity: %w", err)
	}
	defer rows.Close()

	var out []domain.ActivityEvent
	for rows.Next() {
		var (
			at, kind string
			e        = domain.ActivityEvent{Type: domain.ActivityLedger}
		)
		if err := rows.Scan(&at, &e.IssueID, &kind, &e.Field, &e.OldValue, &e.NewValue, &e.IssueSubject); err != nil {
			return nil, fmt.Errorf("scan ledger activity: %w", err)
		}
		e.LedgerKind = domain.LedgerKind(kind)
		if e.At, err = time.Parse(timeFmt, at); err != nil {
			return nil, fmt.Errorf("parse ledger at: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) transcriptEvents() ([]domain.ActivityEvent, error) {
	rows, err := s.db.Query(
		`SELECT t.captured_at, t.id, t.title, t.issue_id, i.subject
		 FROM transcripts t LEFT JOIN issues i ON i.id = t.issue_id`)
	if err != nil {
		return nil, fmt.Errorf("query transcript activity: %w", err)
	}
	defer rows.Close()

	var out []domain.ActivityEvent
	for rows.Next() {
		var (
			at      string
			issueID *string
			subject *string
			e       = domain.ActivityEvent{Type: domain.ActivityTranscript}
		)
		if err := rows.Scan(&at, &e.TranscriptID, &e.Title, &issueID, &subject); err != nil {
			return nil, fmt.Errorf("scan transcript activity: %w", err)
		}
		if issueID != nil {
			e.IssueID = *issueID
		}
		if subject != nil {
			e.IssueSubject = *subject
		}
		if e.At, err = time.Parse(timeFmt, at); err != nil {
			return nil, fmt.Errorf("parse transcript captured_at: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
