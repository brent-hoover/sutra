package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

const timeFmt = time.RFC3339Nano

// CreateIssue inserts an issue and its ledger entries in one transaction.
func (s *Store) CreateIssue(issue domain.Issue, ledger []domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO issues (id, subject, body, type, status, priority, owner, parent_id, deleted_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.ID, issue.Subject, issue.Body, string(issue.Type), string(issue.Status),
		string(issue.Priority), issue.Owner, nullString(issue.ParentID), nullTime(issue.DeletedAt),
		issue.CreatedAt.Format(timeFmt), issue.UpdatedAt.Format(timeFmt),
	); err != nil {
		return fmt.Errorf("insert issue: %w", err)
	}

	for _, e := range ledger {
		if _, err := tx.Exec(
			`INSERT INTO ledger (id, issue_id, at, actor, kind, field, old_value, new_value)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.IssueID, e.At.Format(timeFmt), e.Actor, string(e.Kind), e.Field, e.OldValue, e.NewValue,
		); err != nil {
			return fmt.Errorf("insert ledger: %w", err)
		}
	}

	return tx.Commit()
}

// GetIssue returns the issue with the given id, or ErrNotFound.
func (s *Store) GetIssue(id string) (domain.Issue, error) {
	var (
		issue                 domain.Issue
		typ, status, priority string
		parentID, deletedAt   sql.NullString
		createdAt, updatedAt  string
	)
	err := s.db.QueryRow(
		`SELECT id, subject, body, type, status, priority, owner, parent_id, deleted_at, created_at, updated_at
		 FROM issues WHERE id = ?`, id,
	).Scan(
		&issue.ID, &issue.Subject, &issue.Body, &typ, &status, &priority, &issue.Owner,
		&parentID, &deletedAt, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Issue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("get issue: %w", err)
	}

	issue.Type = domain.IssueType(typ)
	issue.Status = domain.Status(status)
	issue.Priority = domain.Priority(priority)
	if parentID.Valid {
		issue.ParentID = &parentID.String
	}
	if deletedAt.Valid {
		if t, err := time.Parse(timeFmt, deletedAt.String); err == nil {
			issue.DeletedAt = &t
		}
	}
	if issue.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
		return domain.Issue{}, fmt.Errorf("parse created_at: %w", err)
	}
	if issue.UpdatedAt, err = time.Parse(timeFmt, updatedAt); err != nil {
		return domain.Issue{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return issue, nil
}

// LedgerFor returns an issue's ledger entries in chronological order.
func (s *Store) LedgerFor(issueID string) ([]domain.LedgerEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, issue_id, at, actor, kind, field, old_value, new_value
		 FROM ledger WHERE issue_id = ? ORDER BY at`, issueID)
	if err != nil {
		return nil, fmt.Errorf("query ledger: %w", err)
	}
	defer rows.Close()

	var entries []domain.LedgerEntry
	for rows.Next() {
		var (
			e    domain.LedgerEntry
			at   string
			kind string
		)
		if err := rows.Scan(&e.ID, &e.IssueID, &at, &e.Actor, &kind, &e.Field, &e.OldValue, &e.NewValue); err != nil {
			return nil, fmt.Errorf("scan ledger: %w", err)
		}
		e.Kind = domain.LedgerKind(kind)
		if e.At, err = time.Parse(timeFmt, at); err != nil {
			return nil, fmt.Errorf("parse ledger at: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func nullString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(timeFmt)
}
