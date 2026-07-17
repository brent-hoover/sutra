package service

import (
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

// IssueHistory returns an issue's ledger entries in chronological order.
func (s *Service) IssueHistory(id string) ([]domain.LedgerEntry, error) {
	return s.store.LedgerFor(id)
}
