package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateIssue validates input, applies defaults, persists the issue with a
// "created" ledger entry, and returns the stored issue.
func (s *Service) CreateIssue(subject, body string) (domain.Issue, error) {
	now := time.Now().UTC()
	issue := domain.Issue{
		ID:        domain.NewID(),
		Subject:   subject,
		Body:      body,
		Type:      domain.TypeTask,
		Status:    domain.StatusOpen,
		Priority:  domain.P2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := issue.Validate(); err != nil {
		return domain.Issue{}, err
	}

	entry := domain.LedgerEntry{
		ID:      domain.NewID(),
		IssueID: issue.ID,
		At:      now,
		Kind:    domain.LedgerCreated,
	}
	if err := s.store.CreateIssue(issue, []domain.LedgerEntry{entry}); err != nil {
		return domain.Issue{}, err
	}
	return issue, nil
}

// GetIssue returns the issue with the given id.
func (s *Service) GetIssue(id string) (domain.Issue, error) {
	return s.store.GetIssue(id)
}

// ListIssues returns live issues matching the filter (soft-deleted excluded).
func (s *Service) ListIssues(f domain.IssueFilter) ([]domain.Issue, error) {
	return s.store.ListIssues(f)
}

// IssueUpdate carries the fields a caller wants to change. A nil pointer means
// "leave unchanged"; this lets callers clear a field (e.g. owner to "").
type IssueUpdate struct {
	Type     *domain.IssueType
	Status   *domain.Status
	Priority *domain.Priority
	Owner    *string
}

// UpdateIssue applies the requested field changes, appends a ledger entry per
// changed field (kind status_changed for status, updated otherwise), advances
// updated_at, and returns the stored issue. A no-op update touches nothing.
func (s *Service) UpdateIssue(id string, upd IssueUpdate) (domain.Issue, error) {
	issue, err := s.store.GetIssue(id)
	if err != nil {
		return domain.Issue{}, err
	}
	if upd.Type != nil && !upd.Type.Valid() {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("invalid type %q", *upd.Type))
	}
	if upd.Status != nil && !upd.Status.Valid() {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("invalid status %q", *upd.Status))
	}
	if upd.Priority != nil && !upd.Priority.Valid() {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("invalid priority %q", *upd.Priority))
	}

	now := time.Now().UTC()
	var entries []domain.LedgerEntry
	record := func(kind domain.LedgerKind, field, oldVal, newVal string) {
		entries = append(entries, domain.LedgerEntry{
			ID:       domain.NewID(),
			IssueID:  id,
			At:       now,
			Kind:     kind,
			Field:    field,
			OldValue: oldVal,
			NewValue: newVal,
		})
	}

	if upd.Type != nil && *upd.Type != issue.Type {
		old := string(issue.Type)
		issue.Type = *upd.Type
		record(domain.LedgerUpdated, "type", old, string(issue.Type))
	}
	if upd.Status != nil && *upd.Status != issue.Status {
		old := string(issue.Status)
		issue.Status = *upd.Status
		record(domain.LedgerStatusChanged, "status", old, string(issue.Status))
	}
	if upd.Priority != nil && *upd.Priority != issue.Priority {
		old := string(issue.Priority)
		issue.Priority = *upd.Priority
		record(domain.LedgerUpdated, "priority", old, string(issue.Priority))
	}
	if upd.Owner != nil && *upd.Owner != issue.Owner {
		old := issue.Owner
		issue.Owner = *upd.Owner
		record(domain.LedgerUpdated, "owner", old, issue.Owner)
	}

	if len(entries) == 0 {
		return issue, nil // nothing changed
	}
	issue.UpdatedAt = now
	if err := s.store.UpdateIssue(issue, entries); err != nil {
		return domain.Issue{}, err
	}
	return issue, nil
}

// SoftDeleteIssue sets deleted_at (excluding the issue from default lists and
// search), appends a deleted ledger entry, and returns the stored issue.
// Deleting an already-deleted issue is a no-op.
func (s *Service) SoftDeleteIssue(id string) (domain.Issue, error) {
	issue, err := s.store.GetIssue(id)
	if err != nil {
		return domain.Issue{}, err
	}
	if issue.DeletedAt != nil {
		return issue, nil
	}
	now := time.Now().UTC()
	issue.DeletedAt = &now
	issue.UpdatedAt = now
	entry := domain.LedgerEntry{
		ID:      domain.NewID(),
		IssueID: id,
		At:      now,
		Kind:    domain.LedgerDeleted,
	}
	if err := s.store.UpdateIssue(issue, []domain.LedgerEntry{entry}); err != nil {
		return domain.Issue{}, err
	}
	return issue, nil
}

// IssueHistory returns an issue's ledger entries in chronological order.
func (s *Service) IssueHistory(id string) ([]domain.LedgerEntry, error) {
	return s.store.LedgerFor(id)
}
