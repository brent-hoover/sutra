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

	if err := insertLedger(tx, ledger); err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateIssue writes an issue's mutable columns and appends its ledger entries
// in one transaction. Returns ErrNotFound if the issue does not exist.
func (s *Store) UpdateIssue(issue domain.Issue, ledger []domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE issues
		 SET type = ?, status = ?, priority = ?, owner = ?, parent_id = ?, deleted_at = ?, updated_at = ?
		 WHERE id = ?`,
		string(issue.Type), string(issue.Status), string(issue.Priority), issue.Owner,
		nullString(issue.ParentID), nullTime(issue.DeletedAt), issue.UpdatedAt.Format(timeFmt), issue.ID,
	)
	if err != nil {
		return fmt.Errorf("update issue: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}

	if err := insertLedger(tx, ledger); err != nil {
		return err
	}

	return tx.Commit()
}

const issueColumns = `id, subject, body, type, status, priority, owner, parent_id, deleted_at, created_at, updated_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanIssue reads one issue row in issueColumns order.
func scanIssue(sc rowScanner) (domain.Issue, error) {
	var (
		issue                 domain.Issue
		typ, status, priority string
		parentID, deletedAt   sql.NullString
		createdAt, updatedAt  string
	)
	if err := sc.Scan(
		&issue.ID, &issue.Subject, &issue.Body, &typ, &status, &priority, &issue.Owner,
		&parentID, &deletedAt, &createdAt, &updatedAt,
	); err != nil {
		return domain.Issue{}, err
	}
	issue.Type = domain.IssueType(typ)
	issue.Status = domain.Status(status)
	issue.Priority = domain.Priority(priority)
	if parentID.Valid {
		issue.ParentID = &parentID.String
	}
	if deletedAt.Valid {
		t, err := time.Parse(timeFmt, deletedAt.String)
		if err != nil {
			return domain.Issue{}, fmt.Errorf("parse deleted_at: %w", err)
		}
		issue.DeletedAt = &t
	}
	var err error
	if issue.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
		return domain.Issue{}, fmt.Errorf("parse created_at: %w", err)
	}
	if issue.UpdatedAt, err = time.Parse(timeFmt, updatedAt); err != nil {
		return domain.Issue{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return issue, nil
}

// GetIssue returns the issue with the given id, or ErrNotFound.
func (s *Store) GetIssue(id string) (domain.Issue, error) {
	issue, err := scanIssue(s.db.QueryRow(`SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Issue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("get issue: %w", err)
	}
	return issue, nil
}

// ListIssues returns live (non-soft-deleted) issues matching the filter, in
// creation order. Filter fields combine with AND.
func (s *Store) ListIssues(f domain.IssueFilter) ([]domain.Issue, error) {
	q := `SELECT ` + issueColumns + ` FROM issues WHERE deleted_at IS NULL`
	var args []any
	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, string(f.Status))
	}
	if f.Type != "" {
		q += " AND type = ?"
		args = append(args, string(f.Type))
	}
	if f.Priority != "" {
		q += " AND priority = ?"
		args = append(args, string(f.Priority))
	}
	if f.Owner != "" {
		q += " AND owner = ?"
		args = append(args, f.Owner)
	}
	q += " ORDER BY created_at, id"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	var issues []domain.Issue
	for rows.Next() {
		issue, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	return issues, rows.Err()
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

// insertLedger appends append-only ledger entries within a transaction.
func insertLedger(tx *sql.Tx, entries []domain.LedgerEntry) error {
	for _, e := range entries {
		if _, err := tx.Exec(
			`INSERT INTO ledger (id, issue_id, at, actor, kind, field, old_value, new_value)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.IssueID, e.At.Format(timeFmt), e.Actor, string(e.Kind), e.Field, e.OldValue, e.NewValue,
		); err != nil {
			return fmt.Errorf("insert ledger: %w", err)
		}
	}
	return nil
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
