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
	return s.createIssue(subject, body, nil, nil)
}

// CreateIssueInProject creates an issue scoped to a project (projectID may be
// empty for none). The project must exist. Returns ErrNotFound if it is missing.
func (s *Service) CreateIssueInProject(subject, body, projectID string) (domain.Issue, error) {
	var pid *string
	if projectID != "" {
		if _, err := s.store.GetProject(projectID); err != nil {
			return domain.Issue{}, err
		}
		pid = &projectID
	}
	return s.createIssue(subject, body, nil, pid)
}

// CreateChildIssue creates an issue whose parent_id is set to parentID. The
// parent must exist. Because the child is a fresh leaf, this cannot introduce a
// cycle or a self-link, and the issue and its parent link are persisted in the
// one insert (no orphan window). Returns ErrNotFound if the parent is missing.
func (s *Service) CreateChildIssue(subject, body, parentID string) (domain.Issue, error) {
	if _, err := s.store.GetIssue(parentID); err != nil {
		return domain.Issue{}, err
	}
	return s.createIssue(subject, body, &parentID, nil)
}

func (s *Service) createIssue(subject, body string, parentID, projectID *string) (domain.Issue, error) {
	now := time.Now().UTC()
	issue := domain.Issue{
		ID:        domain.NewID(),
		Subject:   subject,
		Body:      body,
		Type:      domain.TypeTask,
		Status:    domain.StatusOpen,
		Priority:  domain.P2,
		ParentID:  parentID,
		ProjectID: projectID,
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

// GetIssue returns the issue with the given id (with its derived labels).
func (s *Service) GetIssue(id string) (domain.Issue, error) {
	return s.store.GetIssue(id)
}

// GetIssueView returns the full read projection of an issue: its fields and
// labels plus its related/blocking links and comments, read as one consistent
// snapshot in the store. Returns ErrNotFound if the issue does not exist. The
// link and comment slices are always non-nil so the JSON response renders them
// as arrays.
func (s *Service) GetIssueView(id string) (domain.IssueView, error) {
	view, err := s.store.GetIssueView(id)
	if err != nil {
		return domain.IssueView{}, err
	}
	view.Related = orEmpty(view.Related)
	view.BlockedBy = orEmpty(view.BlockedBy)
	view.IsBlocking = orEmpty(view.IsBlocking)
	if view.Comments == nil {
		view.Comments = []domain.Comment{}
	}
	return view, nil
}

// orEmpty returns a non-nil slice so JSON renders [] rather than null.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ListIssues returns live issues matching the filter (soft-deleted excluded).
func (s *Service) ListIssues(f domain.IssueFilter) ([]domain.Issue, error) {
	return s.store.ListIssues(f)
}

// IssueUpdate carries the fields a caller wants to change. A nil pointer means
// "leave unchanged"; this lets callers clear a field (e.g. owner to "").
//
// Reparenting is deliberately not exposed here: the parent link is set once at
// creation (CreateChildIssue). Cycle-safe reparenting is Slice 4's concern.
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
	if upd.Type != nil && !upd.Type.Valid() {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("invalid type %q", *upd.Type))
	}
	if upd.Status != nil && !upd.Status.Valid() {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("invalid status %q", *upd.Status))
	}
	if upd.Priority != nil && !upd.Priority.Valid() {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("invalid priority %q", *upd.Priority))
	}

	// Read-modify-write inside one store transaction so concurrent updates
	// can't lose each other's changes or resurrect a soft-deleted issue.
	return s.store.UpdateIssueTx(id, func(issue *domain.Issue) ([]domain.LedgerEntry, error) {
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
			return nil, nil // nothing changed
		}
		issue.UpdatedAt = now
		return entries, nil
	})
}

// SoftDeleteIssue sets deleted_at (excluding the issue from default lists and
// search), appends a deleted ledger entry, and returns the stored issue.
// Deleting an already-deleted issue is a no-op.
func (s *Service) SoftDeleteIssue(id string) (domain.Issue, error) {
	// Read-modify-write inside one store transaction so a concurrent update
	// can't run between the delete decision and the write.
	return s.store.UpdateIssueTx(id, func(issue *domain.Issue) ([]domain.LedgerEntry, error) {
		if issue.DeletedAt != nil {
			return nil, nil // already deleted: no-op
		}
		now := time.Now().UTC()
		issue.DeletedAt = &now
		issue.UpdatedAt = now
		return []domain.LedgerEntry{{
			ID:      domain.NewID(),
			IssueID: id,
			At:      now,
			Kind:    domain.LedgerDeleted,
		}}, nil
	})
}

// IssueHistory returns an issue's ledger entries in chronological order. It
// returns ErrNotFound if the issue does not exist (so callers can 404 rather
// than return an empty history for a bogus id).
func (s *Service) IssueHistory(id string) ([]domain.LedgerEntry, error) {
	if _, err := s.store.GetIssue(id); err != nil {
		return nil, err
	}
	return s.store.LedgerFor(id)
}
