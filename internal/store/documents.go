package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateDocument inserts a document.
func (s *Store) CreateDocument(doc domain.Document) error {
	if _, err := s.db.Exec(
		`INSERT INTO documents (id, issue_id, kind, title, content, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.IssueID, string(doc.Kind), doc.Title, doc.Content,
		doc.CreatedAt.Format(timeFmt), doc.UpdatedAt.Format(timeFmt),
	); err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

// GetDocument returns the document with the given id, or ErrNotFound.
func (s *Store) GetDocument(id string) (domain.Document, error) {
	var (
		doc                  domain.Document
		kind                 string
		createdAt, updatedAt string
	)
	err := s.db.QueryRow(
		`SELECT id, issue_id, kind, title, content, created_at, updated_at
		 FROM documents WHERE id = ?`, id,
	).Scan(&doc.ID, &doc.IssueID, &kind, &doc.Title, &doc.Content, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Document{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Document{}, fmt.Errorf("get document: %w", err)
	}

	doc.Kind = domain.DocumentKind(kind)
	if doc.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
		return domain.Document{}, fmt.Errorf("parse created_at: %w", err)
	}
	if doc.UpdatedAt, err = time.Parse(timeFmt, updatedAt); err != nil {
		return domain.Document{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return doc, nil
}

// DocumentsForIssue returns an issue's documents, oldest first.
func (s *Store) DocumentsForIssue(issueID string) ([]domain.Document, error) {
	rows, err := s.db.Query(
		`SELECT id, issue_id, kind, title, content, created_at, updated_at
		 FROM documents WHERE issue_id = ? ORDER BY created_at, id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("query documents: %w", err)
	}
	defer rows.Close()

	var docs []domain.Document
	for rows.Next() {
		var (
			doc                  domain.Document
			kind                 string
			createdAt, updatedAt string
		)
		if err := rows.Scan(&doc.ID, &doc.IssueID, &kind, &doc.Title, &doc.Content, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		doc.Kind = domain.DocumentKind(kind)
		if doc.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if doc.UpdatedAt, err = time.Parse(timeFmt, updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

// UpdateDocument writes new content and updated_at for the document, returning
// ErrNotFound if it does not exist.
func (s *Store) UpdateDocument(id, content string, updatedAt time.Time) error {
	res, err := s.db.Exec(
		`UPDATE documents SET content = ?, updated_at = ? WHERE id = ?`,
		content, updatedAt.Format(timeFmt), id,
	)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update document rows: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteDocument removes the document, returning ErrNotFound if it does not exist.
func (s *Store) DeleteDocument(id string) error {
	res, err := s.db.Exec(`DELETE FROM documents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete document rows: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
