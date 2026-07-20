package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateThread creates a thread, optionally scoped to a project (projectID may
// be empty). The project, if given, must exist (ErrNotFound otherwise).
func (s *Service) CreateThread(title, body, projectID string) (domain.Thread, error) {
	var pid *string
	if projectID != "" {
		if _, err := s.store.GetProject(projectID); err != nil {
			return domain.Thread{}, err
		}
		pid = &projectID
	}
	now := time.Now().UTC()
	t := domain.Thread{
		ID:        domain.NewID(),
		ProjectID: pid,
		Title:     title,
		Body:      body,
		Status:    domain.ThreadActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := t.Validate(); err != nil {
		return domain.Thread{}, err
	}
	if err := s.store.CreateThread(t); err != nil {
		return domain.Thread{}, err
	}
	return t, nil
}

// GetThread returns a thread by id (ErrNotFound if missing).
func (s *Service) GetThread(id string) (domain.Thread, error) {
	return s.store.GetThread(id)
}

// GetThreadView returns a thread with its members. Returns ErrNotFound if the
// thread does not exist.
func (s *Service) GetThreadView(id string) (domain.ThreadView, error) {
	t, err := s.store.GetThread(id)
	if err != nil {
		return domain.ThreadView{}, err
	}
	items, err := s.store.ThreadItems(id)
	if err != nil {
		return domain.ThreadView{}, err
	}
	if items == nil {
		items = []domain.ThreadItem{}
	}
	return domain.ThreadView{Thread: t, Items: items}, nil
}

// ListThreads returns threads, optionally restricted to a project.
func (s *Service) ListThreads(projectID string) ([]domain.Thread, error) {
	return s.store.ListThreads(projectID)
}

// ThreadUpdate carries the mutable fields of a thread; nil fields are unchanged.
// A thread's project is set at creation and not reassigned here.
type ThreadUpdate struct {
	Title  *string
	Body   *string
	Status *domain.ThreadStatus
}

// UpdateThread applies the non-nil fields of upd. Returns ErrNotFound if the
// thread does not exist, or ErrInvalidThread on a bad status.
func (s *Service) UpdateThread(id string, upd ThreadUpdate) (domain.Thread, error) {
	return s.store.UpdateThreadTx(id, func(t *domain.Thread) error {
		if upd.Title != nil {
			t.Title = *upd.Title
		}
		if upd.Body != nil {
			t.Body = *upd.Body
		}
		if upd.Status != nil {
			t.Status = *upd.Status
		}
		return nil
	})
}

// DeleteThread removes a thread and its memberships (members are untouched).
func (s *Service) DeleteThread(id string) error {
	return s.store.DeleteThread(id)
}

// AddThreadItem attaches an item of the given kind to a thread. The kind must be
// known and both the thread and the item must exist (ErrNotFound otherwise).
func (s *Service) AddThreadItem(threadID string, kind domain.ThreadItemKind, itemID string) error {
	if !kind.Valid() {
		return errors.Join(domain.ErrInvalidThread, fmt.Errorf("invalid item kind %q", kind))
	}
	return s.store.AddThreadItem(threadID, kind, itemID)
}

// RemoveThreadItem detaches an item from a thread (idempotent).
func (s *Service) RemoveThreadItem(threadID string, kind domain.ThreadItemKind, itemID string) error {
	return s.store.RemoveThreadItem(threadID, kind, itemID)
}
