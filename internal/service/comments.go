package service

import (
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CommentIssue stores a comment on an existing issue and appends a commented
// ledger entry. Returns ErrNotFound if the issue does not exist and
// ErrInvalidComment if the comment fails validation.
func (s *Service) CommentIssue(issueID, author, body string) (domain.Comment, error) {
	if _, err := s.store.GetIssue(issueID); err != nil {
		return domain.Comment{}, err
	}
	now := time.Now().UTC()
	comment := domain.Comment{
		ID:        domain.NewID(),
		IssueID:   issueID,
		Author:    author,
		Body:      body,
		CreatedAt: now,
	}
	if err := comment.Validate(); err != nil {
		return domain.Comment{}, err
	}
	entry := domain.LedgerEntry{
		ID:      domain.NewID(),
		IssueID: issueID,
		At:      now,
		Actor:   author,
		Kind:    domain.LedgerCommented,
	}
	if err := s.store.CreateComment(comment, []domain.LedgerEntry{entry}); err != nil {
		return domain.Comment{}, err
	}
	return comment, nil
}

// IssueComments returns an issue's comments in chronological order. It returns
// ErrNotFound if the issue does not exist (mirroring documents, transcripts,
// and history) so callers can 404 rather than return an empty list for a bogus
// id.
func (s *Service) IssueComments(issueID string) ([]domain.Comment, error) {
	if _, err := s.store.GetIssue(issueID); err != nil {
		return nil, err
	}
	return s.store.CommentsFor(issueID)
}
