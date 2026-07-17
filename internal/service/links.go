package service

import (
	"errors"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// SetParent sets childID's parent to parentID, rejecting a self-parent or a
// link that would create a cycle (checked transactionally in the store). It
// appends a "linked" ledger entry to the child and returns the updated issue.
func (s *Service) SetParent(childID, parentID string) (domain.Issue, error) {
	return s.store.SetParent(childID, parentID, domain.LedgerEntry{
		ID:       domain.NewID(),
		IssueID:  childID,
		Kind:     domain.LedgerLinked,
		Field:    "parent_id",
		NewValue: parentID,
	})
}

// RelateIssues records a symmetric "related" link between two issues, appends a
// "linked" ledger entry to the acting issue, and returns it. Relating an issue
// to itself is rejected.
func (s *Service) RelateIssues(issueID, relatedID string) (domain.Issue, error) {
	if issueID == relatedID {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("an issue cannot be related to itself"))
	}
	if err := s.store.AddRelation(issueID, relatedID, domain.LedgerEntry{
		ID:       domain.NewID(),
		IssueID:  issueID,
		At:       time.Now().UTC(),
		Kind:     domain.LedgerLinked,
		Field:    "related",
		NewValue: relatedID,
	}); err != nil {
		return domain.Issue{}, err
	}
	return s.store.GetIssue(issueID)
}

// BlockIssue records that blockerID blocks blockedID, appends a "linked" ledger
// entry to the blocked issue, and returns it. An issue cannot block itself.
func (s *Service) BlockIssue(blockedID, blockerID string) (domain.Issue, error) {
	if blockedID == blockerID {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("an issue cannot block itself"))
	}
	if err := s.store.AddBlock(blockerID, blockedID, domain.LedgerEntry{
		ID:       domain.NewID(),
		IssueID:  blockedID,
		At:       time.Now().UTC(),
		Kind:     domain.LedgerLinked,
		Field:    "blocked_by",
		NewValue: blockerID,
	}); err != nil {
		return domain.Issue{}, err
	}
	return s.store.GetIssue(blockedID)
}
