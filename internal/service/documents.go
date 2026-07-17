package service

import (
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// AttachDocument validates input, confirms the issue exists, persists the
// document with timestamps set, and returns the stored document.
func (s *Service) AttachDocument(issueID string, kind domain.DocumentKind, title, content string) (domain.Document, error) {
	if _, err := s.store.GetIssue(issueID); err != nil {
		return domain.Document{}, err
	}
	now := time.Now().UTC()
	doc := domain.Document{
		ID:        domain.NewID(),
		IssueID:   issueID,
		Kind:      kind,
		Title:     title,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := doc.Validate(); err != nil {
		return domain.Document{}, err
	}
	if err := s.store.CreateDocument(doc); err != nil {
		return domain.Document{}, err
	}
	return doc, nil
}

// GetDocument returns the document with the given id.
func (s *Service) GetDocument(id string) (domain.Document, error) {
	return s.store.GetDocument(id)
}

// ListDocuments returns an issue's documents.
func (s *Service) ListDocuments(issueID string) ([]domain.Document, error) {
	if _, err := s.store.GetIssue(issueID); err != nil {
		return nil, err
	}
	return s.store.DocumentsForIssue(issueID)
}

// UpdateDocument stores new content and advances updated_at, returning the
// updated document.
func (s *Service) UpdateDocument(id, content string) (domain.Document, error) {
	if content == "" {
		return domain.Document{}, domain.ErrInvalidDocument
	}
	return s.store.UpdateDocument(id, content)
}

// RemoveDocument deletes the document.
func (s *Service) RemoveDocument(id string) error {
	return s.store.DeleteDocument(id)
}
