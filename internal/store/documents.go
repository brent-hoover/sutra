package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateDocument inserts a document and advances the owning issue's updated_at
// in the same transaction (attaching a document is a change to the issue).
func (s *Store) CreateDocument(doc domain.Document) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO documents (id, issue_id, kind, title, content, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.IssueID, string(doc.Kind), doc.Title, doc.Content,
		doc.CreatedAt.Format(timeFmt), doc.UpdatedAt.Format(timeFmt),
	); err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE issues SET updated_at = ? WHERE id = ?`,
		now.Format(timeFmt), doc.IssueID,
	); err != nil {
		return fmt.Errorf("bump issue updated_at: %w", err)
	}
	if err := insertLedger(tx, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: doc.IssueID, At: now,
		Kind: domain.LedgerUpdated, Field: "document", NewValue: "attached " + string(doc.Kind),
	}}); err != nil {
		return err
	}
	return tx.Commit()
}

const documentColumns = `id, issue_id, kind, title, content, created_at, updated_at`

// scanDocument reads one document row in documentColumns order.
func scanDocument(row rowScanner) (domain.Document, error) {
	var (
		doc                  domain.Document
		kind                 string
		createdAt, updatedAt string
	)
	err := row.Scan(&doc.ID, &doc.IssueID, &kind, &doc.Title, &doc.Content, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Document{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Document{}, fmt.Errorf("scan document: %w", err)
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

// GetDocument returns the document with the given id, or ErrNotFound.
func (s *Store) GetDocument(id string) (domain.Document, error) {
	return scanDocument(s.db.QueryRow(
		`SELECT `+documentColumns+` FROM documents WHERE id = ?`, id))
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

// UpdateDocument writes new content and advances updated_at for the document,
// returning ErrNotFound if it does not exist. The timestamp is generated inside
// the write transaction so that, under concurrent updates serialized on the
// single connection, the last write always carries the latest updated_at.
func (s *Store) UpdateDocument(id, content string) (domain.Document, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Document{}, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE documents SET content = ?, updated_at = ? WHERE id = ?`,
		content, now.Format(timeFmt), id,
	)
	if err != nil {
		return domain.Document{}, fmt.Errorf("update document: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return domain.Document{}, fmt.Errorf("update document rows: %w", err)
	}
	if n == 0 {
		return domain.Document{}, domain.ErrNotFound
	}
	// Read the row back inside the same transaction so the returned document
	// reflects exactly this update, never a concurrently-committed one.
	doc, err := scanDocument(tx.QueryRow(
		`SELECT `+documentColumns+` FROM documents WHERE id = ?`, id))
	if err != nil {
		return domain.Document{}, err
	}
	// Updating a document is a change to the owning issue.
	if _, err := tx.Exec(
		`UPDATE issues SET updated_at = ? WHERE id = ?`,
		now.Format(timeFmt), doc.IssueID,
	); err != nil {
		return domain.Document{}, fmt.Errorf("bump issue updated_at: %w", err)
	}
	if err := insertLedger(tx, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: doc.IssueID, At: now,
		Kind: domain.LedgerUpdated, Field: "document", NewValue: "updated " + string(doc.Kind),
	}}); err != nil {
		return domain.Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Document{}, err
	}
	return doc, nil
}

// DeleteDocument removes the document and advances the owning issue's
// updated_at in the same transaction, returning ErrNotFound if it does not exist.
func (s *Store) DeleteDocument(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var issueID string
	if err := tx.QueryRow(`SELECT issue_id FROM documents WHERE id = ?`, id).Scan(&issueID); errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lookup document: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM documents WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	// A hard-deleted document must not leave dangling thread memberships (the
	// invariant is that attached items exist), so remove them in the same tx.
	if _, err := tx.Exec(`DELETE FROM thread_item WHERE kind = 'document' AND item_id = ?`, id); err != nil {
		return fmt.Errorf("detach document from threads: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE issues SET updated_at = ? WHERE id = ?`,
		now.Format(timeFmt), issueID,
	); err != nil {
		return fmt.Errorf("bump issue updated_at: %w", err)
	}
	if err := insertLedger(tx, []domain.LedgerEntry{{
		ID: domain.NewID(), IssueID: issueID, At: now,
		Kind: domain.LedgerUpdated, Field: "document", OldValue: "removed " + id,
	}}); err != nil {
		return err
	}
	return tx.Commit()
}
