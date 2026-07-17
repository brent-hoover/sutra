package store

import (
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateComment inserts a comment and its ledger entries in one transaction.
func (s *Store) CreateComment(c domain.Comment, ledger []domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO comments (id, issue_id, author, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.IssueID, c.Author, c.Body, c.CreatedAt.Format(timeFmt),
	); err != nil {
		return fmt.Errorf("insert comment: %w", err)
	}

	// A comment is a change to the issue: advance its updated_at in the same tx.
	if _, err := tx.Exec(
		`UPDATE issues SET updated_at = ? WHERE id = ?`,
		c.CreatedAt.Format(timeFmt), c.IssueID,
	); err != nil {
		return fmt.Errorf("bump issue updated_at: %w", err)
	}

	if err := insertLedger(tx, ledger); err != nil {
		return err
	}

	return tx.Commit()
}

// CommentsFor returns an issue's comments in chronological order.
func (s *Store) CommentsFor(issueID string) ([]domain.Comment, error) {
	rows, err := s.db.Query(
		`SELECT id, issue_id, author, body, created_at
		 FROM comments WHERE issue_id = ? ORDER BY created_at, id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()

	var comments []domain.Comment
	for rows.Next() {
		var (
			c         domain.Comment
			createdAt string
		)
		if err := rows.Scan(&c.ID, &c.IssueID, &c.Author, &c.Body, &createdAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		if c.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
			return nil, fmt.Errorf("parse comment created_at: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}
