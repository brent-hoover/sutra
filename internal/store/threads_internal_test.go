package store

import (
	"errors"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

func newThread(t *testing.T, s *Store, title string) domain.Thread {
	t.Helper()
	now := time.Now().UTC()
	th := domain.Thread{ID: domain.NewID(), Title: title, Status: domain.ThreadActive, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateThread(th); err != nil {
		t.Fatalf("create thread: %v", err)
	}
	return th
}

func newIssue(t *testing.T, s *Store, subject string) domain.Issue {
	t.Helper()
	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: subject, Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(iss, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	return iss
}

// GetThreadView reads thread and members atomically; a missing thread is
// ErrNotFound (never a phantom thread with empty members).
func TestGetThreadView(t *testing.T) {
	s := openStore(t)
	if _, _, err := s.GetThreadView("missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing thread: err = %v, want ErrNotFound", err)
	}

	th := newThread(t, s, "t")
	iss := newIssue(t, s, "member")
	if err := s.AddThreadItem(th.ID, domain.ThreadItemIssue, iss.ID); err != nil {
		t.Fatalf("attach: %v", err)
	}
	got, items, err := s.GetThreadView(th.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if got.ID != th.ID || len(items) != 1 || items[0].ItemID != iss.ID {
		t.Errorf("unexpected view: %+v items=%+v", got, items)
	}
}

// RemoveThreadItem verifies the thread exists and is idempotent for an absent
// membership.
func TestRemoveThreadItem(t *testing.T) {
	s := openStore(t)
	if err := s.RemoveThreadItem("missing", domain.ThreadItemIssue, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing thread: err = %v, want ErrNotFound", err)
	}
	th := newThread(t, s, "t")
	if err := s.RemoveThreadItem(th.ID, domain.ThreadItemIssue, "never-added"); err != nil {
		t.Errorf("idempotent remove of absent membership: %v", err)
	}
}
