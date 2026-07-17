package service

import (
	"errors"

	"github.com/brent-hoover/sutra/internal/domain"
)

// AddLabel adds a free-text label to an issue, appends an "updated" ledger
// entry, and returns the updated issue. An empty label is rejected.
func (s *Service) AddLabel(issueID, label string) (domain.Issue, error) {
	if label == "" {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("label is required"))
	}
	if err := s.store.AddLabel(issueID, label, domain.LedgerEntry{
		ID:       domain.NewID(),
		IssueID:  issueID,
		Kind:     domain.LedgerUpdated,
		Field:    "label",
		NewValue: label,
	}); err != nil {
		return domain.Issue{}, err
	}
	return s.store.GetIssue(issueID)
}

// RemoveLabel removes a label from an issue, appends an "updated" ledger entry,
// and returns the updated issue. An empty label is rejected.
func (s *Service) RemoveLabel(issueID, label string) (domain.Issue, error) {
	if label == "" {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("label is required"))
	}
	if err := s.store.RemoveLabel(issueID, label, domain.LedgerEntry{
		ID:       domain.NewID(),
		IssueID:  issueID,
		Kind:     domain.LedgerUpdated,
		Field:    "label",
		OldValue: label,
	}); err != nil {
		return domain.Issue{}, err
	}
	return s.store.GetIssue(issueID)
}
