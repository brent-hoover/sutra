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

	// Validate references in the same transaction as the insert, so a concurrent
	// delete of the parent or project can't leave a dangling reference.
	if issue.ParentID != nil {
		if err := existsInTx(tx, "issues", *issue.ParentID); err != nil {
			return err
		}
	}
	if issue.ProjectID != nil {
		if err := existsInTx(tx, "projects", *issue.ProjectID); err != nil {
			return err
		}
	}

	if err := insertIssueTx(tx, issue); err != nil {
		return err
	}

	if err := insertLedger(tx, ledger); err != nil {
		return err
	}

	return tx.Commit()
}

// insertIssueTx inserts one issue row within an existing transaction. Shared by
// CreateIssue and BuildPlan so the column list lives in exactly one place.
func insertIssueTx(tx *sql.Tx, issue domain.Issue) error {
	if _, err := tx.Exec(
		`INSERT INTO issues (id, subject, body, type, status, priority, owner, parent_id, project_id, approval, deleted_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.ID, issue.Subject, issue.Body, string(issue.Type), string(issue.Status),
		string(issue.Priority), issue.Owner, nullString(issue.ParentID), nullString(issue.ProjectID),
		string(issue.Approval), nullTime(issue.DeletedAt), issue.CreatedAt.Format(timeFmt), issue.UpdatedAt.Format(timeFmt),
	); err != nil {
		return fmt.Errorf("insert issue: %w", err)
	}
	return nil
}

// UpdateIssueTx reads the issue, hands it to mutate for in-place modification,
// then writes the mutated columns and any ledger entries mutate returned — all
// in one transaction, so the read-modify-write is atomic (no lost updates, no
// resurrecting a concurrently soft-deleted issue). If mutate returns no ledger
// entries the row is left untouched and the (unchanged) issue is returned.
// Returns ErrNotFound if the issue does not exist, or any error mutate returns.
func (s *Store) UpdateIssueTx(id string, mutate func(issue *domain.Issue) ([]domain.LedgerEntry, error)) (domain.Issue, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Issue{}, err
	}
	defer tx.Rollback()

	issue, err := scanIssue(tx.QueryRow(`SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Issue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("get issue: %w", err)
	}
	// Hydrate labels so update/delete responses carry the derived labels
	// projection, consistent with GetIssue/ListIssues.
	if issue.Labels, err = labelsFor(tx, id); err != nil {
		return domain.Issue{}, err
	}

	ledger, err := mutate(&issue)
	if err != nil {
		return domain.Issue{}, err
	}
	if len(ledger) == 0 {
		return issue, nil // nothing changed
	}

	if _, err := tx.Exec(
		`UPDATE issues
		 SET type = ?, status = ?, priority = ?, owner = ?, parent_id = ?, deleted_at = ?, updated_at = ?
		 WHERE id = ?`,
		string(issue.Type), string(issue.Status), string(issue.Priority), issue.Owner,
		nullString(issue.ParentID), nullTime(issue.DeletedAt), issue.UpdatedAt.Format(timeFmt), issue.ID,
	); err != nil {
		return domain.Issue{}, fmt.Errorf("update issue: %w", err)
	}

	if err := insertLedger(tx, ledger); err != nil {
		return domain.Issue{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Issue{}, err
	}
	return issue, nil
}

const issueColumns = `id, subject, body, type, status, priority, owner, parent_id, project_id, approval, deleted_at, created_at, updated_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanIssue reads one issue row in issueColumns order.
func scanIssue(sc rowScanner) (domain.Issue, error) {
	var (
		issue                 domain.Issue
		typ, status, priority string
		approval              string
		parentID, projectID   sql.NullString
		deletedAt             sql.NullString
		createdAt, updatedAt  string
	)
	if err := sc.Scan(
		&issue.ID, &issue.Subject, &issue.Body, &typ, &status, &priority, &issue.Owner,
		&parentID, &projectID, &approval, &deletedAt, &createdAt, &updatedAt,
	); err != nil {
		return domain.Issue{}, err
	}
	issue.Type = domain.IssueType(typ)
	issue.Status = domain.Status(status)
	issue.Priority = domain.Priority(priority)
	issue.Approval = domain.Approval(approval)
	if parentID.Valid {
		issue.ParentID = &parentID.String
	}
	if projectID.Valid {
		issue.ProjectID = &projectID.String
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

// GetIssue returns the issue with the given id (with its derived labels), or
// ErrNotFound.
func (s *Store) GetIssue(id string) (domain.Issue, error) {
	// Read the row and its labels in one transaction so a concurrent label
	// mutation can't yield an updated_at/labels combination that never existed.
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Issue{}, err
	}
	defer tx.Rollback()

	issue, err := scanIssue(tx.QueryRow(`SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Issue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("get issue: %w", err)
	}
	if afterIssueReadHook != nil {
		afterIssueReadHook()
	}
	if issue.Labels, err = labelsFor(tx, id); err != nil {
		return domain.Issue{}, err
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
	if f.Label != "" {
		q += " AND id IN (SELECT issue_id FROM issue_label WHERE label = ?)"
		args = append(args, f.Label)
	}
	if f.ProjectID != "" {
		q += " AND project_id = ?"
		args = append(args, f.ProjectID)
	}
	if f.ParentID != "" {
		q += " AND parent_id = ?"
		args = append(args, f.ParentID)
	}
	q += " ORDER BY created_at, id"

	// One read transaction for the row set and every issue's labels, so the
	// list is a consistent snapshot: a concurrent label mutation can't make a
	// hydrated issue's labels disagree with the filter it matched on.
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	var issues []domain.Issue
	for rows.Next() {
		issue, err := scanIssue(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close() // close before the label queries reuse the transaction

	if afterIssueReadHook != nil {
		afterIssueReadHook()
	}
	for i := range issues {
		if issues[i].Labels, err = labelsFor(tx, issues[i].ID); err != nil {
			return nil, err
		}
	}
	return issues, nil
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
