package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

func (s *Store) migrateLabels() error {
	const schema = `
CREATE TABLE IF NOT EXISTS issue_label (
    issue_id TEXT NOT NULL,
    label    TEXT NOT NULL,
    PRIMARY KEY (issue_id, label)
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate labels: %w", err)
	}
	return nil
}

// LabelsForIssue returns an issue's labels in alphabetical order.
func (s *Store) LabelsForIssue(issueID string) ([]string, error) {
	return labelsFor(s.db, issueID)
}

// labelsFor reads an issue's labels through the given querier (db or tx).
func labelsFor(q querier, issueID string) ([]string, error) {
	rows, err := q.Query(`SELECT label FROM issue_label WHERE issue_id = ? ORDER BY label`, issueID)
	if err != nil {
		return nil, fmt.Errorf("query labels: %w", err)
	}
	defer rows.Close()

	var labels []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// AddLabel adds a label to an issue and, in the same transaction, advances the
// issue's updated_at and appends the ledger entry (the timestamp is generated
// inside the tx so it cannot regress). Adding a label already present is a
// no-op: no ledger entry and no updated_at bump. Returns ErrNotFound if the
// issue does not exist.
func (s *Store) AddLabel(issueID, label string, entry domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := issueExistsTx(tx, issueID); err != nil {
		return err
	}
	res, err := tx.Exec(
		`INSERT OR IGNORE INTO issue_label (issue_id, label) VALUES (?, ?)`, issueID, label)
	if err != nil {
		return fmt.Errorf("insert label: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("insert label rows: %w", err)
	}
	if n == 0 {
		return tx.Commit() // already present: idempotent no-op
	}
	if err := bumpIssueAndLog(tx, issueID, entry); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveLabel removes a label from an issue and, in the same transaction,
// advances the issue's updated_at and appends the ledger entry. Removing a
// label the issue does not carry is a no-op. Returns ErrNotFound if the issue
// does not exist.
func (s *Store) RemoveLabel(issueID, label string, entry domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := issueExistsTx(tx, issueID); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM issue_label WHERE issue_id = ? AND label = ?`, issueID, label)
	if err != nil {
		return fmt.Errorf("delete label: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete label rows: %w", err)
	}
	if n == 0 {
		return tx.Commit() // not present: no-op
	}
	if err := bumpIssueAndLog(tx, issueID, entry); err != nil {
		return err
	}
	return tx.Commit()
}

// issueExistsTx returns ErrNotFound if the issue does not exist within tx.
func issueExistsTx(tx *sql.Tx, issueID string) error {
	var one int
	err := tx.QueryRow(`SELECT 1 FROM issues WHERE id = ?`, issueID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup issue: %w", err)
	}
	return nil
}

// bumpIssueAndLog advances an issue's updated_at and appends one ledger entry in
// the caller's transaction, stamping both with a timestamp generated here so it
// cannot regress below a concurrently-committed change.
func bumpIssueAndLog(tx *sql.Tx, issueID string, entry domain.LedgerEntry) error {
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE issues SET updated_at = ? WHERE id = ?`, now.Format(timeFmt), issueID,
	); err != nil {
		return fmt.Errorf("bump issue updated_at: %w", err)
	}
	entry.At = now
	return insertLedger(tx, []domain.LedgerEntry{entry})
}
