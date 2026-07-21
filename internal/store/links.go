package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

func (s *Store) migrateLinks() error {
	const schema = `
CREATE TABLE IF NOT EXISTS issue_relation (
    issue_id         TEXT NOT NULL,
    related_issue_id TEXT NOT NULL,
    PRIMARY KEY (issue_id, related_issue_id)
);
CREATE TABLE IF NOT EXISTS issue_block (
    blocker_id TEXT NOT NULL,
    blocked_id TEXT NOT NULL,
    PRIMARY KEY (blocker_id, blocked_id)
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate links: %w", err)
	}
	return nil
}

// SetParent sets an issue's parent_id and, in one transaction, advances its
// updated_at and appends the ledger entry (stamped inside the tx). It rejects a
// self-parent and any parent that would create a cycle in the parent/child tree
// — the parent chain is walked within the transaction, so the check is atomic
// with the write. Returns ErrNotFound if the child or the parent is missing and
// ErrInvalidIssue for a self-parent or cycle. The entry's OldValue is filled
// with the prior parent id.
func (s *Store) SetParent(childID, parentID string, entry domain.LedgerEntry) (domain.Issue, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Issue{}, err
	}
	defer tx.Rollback()

	if parentID == "" {
		// Setting a parent references an existing issue; an empty id is not a
		// valid parent (clearing a parent is not a supported operation).
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("parent_id is required"))
	}
	if childID == parentID {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("an issue cannot be its own parent"))
	}

	// The child must exist; capture its current parent for the ledger old_value.
	var oldParent sql.NullString
	err = tx.QueryRow(`SELECT parent_id FROM issues WHERE id = ?`, childID).Scan(&oldParent)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Issue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("lookup child: %w", err)
	}
	if oldParent.Valid && oldParent.String == parentID {
		// Parent unchanged: idempotent no-op — no updated_at bump, no ledger entry,
		// matching the relation/block/label operations. Release the connection
		// (held by this tx) before reading the issue back.
		_ = tx.Rollback()
		return s.GetIssue(childID)
	}

	// Walk up from the proposed parent: if the chain reaches the child, the link
	// would close a cycle. The first hop also proves the parent exists.
	cur := parentID
	visited := map[string]bool{}
	for cur != "" {
		if cur == childID {
			return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("parent link would create a cycle"))
		}
		if visited[cur] {
			break // guard against a pre-existing cycle in stored data
		}
		visited[cur] = true
		var next sql.NullString
		err := tx.QueryRow(`SELECT parent_id FROM issues WHERE id = ?`, cur).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Issue{}, domain.ErrNotFound // parent id does not exist
		}
		if err != nil {
			return domain.Issue{}, fmt.Errorf("walk parent chain: %w", err)
		}
		if !next.Valid {
			break
		}
		cur = next.String
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE issues SET parent_id = ?, tracer_order = NULL, updated_at = ? WHERE id = ?`,
		parentID, now.Format(timeFmt), childID,
	); err != nil {
		return domain.Issue{}, fmt.Errorf("set parent: %w", err)
	}
	entry.At = now
	entry.OldValue = oldParent.String
	if err := insertLedger(tx, []domain.LedgerEntry{entry}); err != nil {
		return domain.Issue{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Issue{}, err
	}
	return s.GetIssue(childID)
}

// AddRelation records a symmetric "related" link between two issues and, in one
// transaction, advances both issues' updated_at and appends the ledger entry to
// the acting issue (entry.IssueID). The pair is stored once in canonical order,
// so relating in either direction yields a single edge. Returns ErrNotFound if
// either issue is missing.
func (s *Store) AddRelation(issueID, relatedID string, entry domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := issueExistsTx(tx, issueID); err != nil {
		return err
	}
	if err := issueExistsTx(tx, relatedID); err != nil {
		return err
	}

	// Canonical order keeps the relation a true set (one row per unordered pair).
	a, b := issueID, relatedID
	if a > b {
		a, b = b, a
	}
	res, err := tx.Exec(
		`INSERT OR IGNORE INTO issue_relation (issue_id, related_issue_id) VALUES (?, ?)`, a, b)
	if err != nil {
		return fmt.Errorf("insert relation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("insert relation rows: %w", err)
	}
	if n == 0 {
		return tx.Commit() // already related: idempotent no-op
	}
	if err := bumpTwoAndLog(tx, issueID, relatedID, entry); err != nil {
		return err
	}
	return tx.Commit()
}

// AddBlock records a directional block (blocker blocks blocked) and, in one
// transaction, advances both issues' updated_at and appends the ledger entry to
// the acting issue (entry.IssueID). Returns ErrNotFound if either issue is
// missing.
func (s *Store) AddBlock(blockerID, blockedID string, entry domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := issueExistsTx(tx, blockerID); err != nil {
		return err
	}
	if err := issueExistsTx(tx, blockedID); err != nil {
		return err
	}
	res, err := tx.Exec(
		`INSERT OR IGNORE INTO issue_block (blocker_id, blocked_id) VALUES (?, ?)`, blockerID, blockedID)
	if err != nil {
		return fmt.Errorf("insert block: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("insert block rows: %w", err)
	}
	if n == 0 {
		return tx.Commit() // already blocking: idempotent no-op
	}
	if err := bumpTwoAndLog(tx, blockerID, blockedID, entry); err != nil {
		return err
	}
	return tx.Commit()
}

// Link queries; shared between the standalone accessors and the transactional
// view read so both stay in sync.
const (
	relatedQuery = `SELECT related_issue_id FROM issue_relation WHERE issue_id = ?
	 UNION
	 SELECT issue_id FROM issue_relation WHERE related_issue_id = ?
	 ORDER BY 1`
	blockedByQuery  = `SELECT blocker_id FROM issue_block WHERE blocked_id = ? ORDER BY 1`
	isBlockingQuery = `SELECT blocked_id FROM issue_block WHERE blocker_id = ? ORDER BY 1`
)

// RelatedFor returns the ids related to an issue (both directions), sorted.
func (s *Store) RelatedFor(issueID string) ([]string, error) {
	return idsFor(s.db, relatedQuery, issueID, issueID)
}

// BlockedByFor returns the ids of issues blocking this one (incoming edges).
func (s *Store) BlockedByFor(issueID string) ([]string, error) {
	return idsFor(s.db, blockedByQuery, issueID)
}

// IsBlockingFor returns the ids of issues this one blocks (outgoing edges).
func (s *Store) IsBlockingFor(issueID string) ([]string, error) {
	return idsFor(s.db, isBlockingQuery, issueID)
}

// afterIssueReadHook, if non-nil, runs inside GetIssueView after the issue row
// has been read but before its derived collections. It is a test-only seam
// (always nil in production) that lets a test deterministically commit a write
// between those reads to prove the surrounding transaction isolates the view.
var afterIssueReadHook func()

// GetIssueView reads an issue and its derived labels, related/blocking links,
// and comments within a single transaction, so the projection is a consistent
// snapshot even under concurrent mutations. Returns ErrNotFound if the issue
// does not exist.
func (s *Store) GetIssueView(id string) (domain.IssueView, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.IssueView{}, err
	}
	defer tx.Rollback()

	issue, err := scanIssue(tx.QueryRow(`SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.IssueView{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.IssueView{}, fmt.Errorf("get issue: %w", err)
	}
	if afterIssueReadHook != nil {
		afterIssueReadHook()
	}
	view := domain.IssueView{Issue: issue}
	if view.Issue.Labels, err = labelsFor(tx, id); err != nil {
		return domain.IssueView{}, err
	}
	if view.Related, err = idsFor(tx, relatedQuery, id, id); err != nil {
		return domain.IssueView{}, err
	}
	if view.BlockedBy, err = idsFor(tx, blockedByQuery, id); err != nil {
		return domain.IssueView{}, err
	}
	if view.IsBlocking, err = idsFor(tx, isBlockingQuery, id); err != nil {
		return domain.IssueView{}, err
	}
	if view.Comments, err = commentsFor(tx, id); err != nil {
		return domain.IssueView{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.IssueView{}, err
	}
	return view, nil
}

// idsFor runs an id-list query through the given querier (db or tx).
func idsFor(q querier, query string, args ...any) ([]string, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// bumpTwoAndLog advances both issues' updated_at and appends one ledger entry to
// the acting issue in the caller's transaction, stamping all with a timestamp
// generated here (so it cannot regress). Both endpoints of a link change, so
// both timestamps advance; the ledger entry records the change on entry.IssueID.
func bumpTwoAndLog(tx *sql.Tx, a, b string, entry domain.LedgerEntry) error {
	now := time.Now().UTC()
	for _, id := range []string{a, b} {
		if _, err := tx.Exec(
			`UPDATE issues SET updated_at = ? WHERE id = ?`, now.Format(timeFmt), id,
		); err != nil {
			return fmt.Errorf("bump issue updated_at: %w", err)
		}
	}
	entry.At = now
	return insertLedger(tx, []domain.LedgerEntry{entry})
}
