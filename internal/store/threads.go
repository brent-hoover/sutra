package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

const threadColumns = `id, project_id, title, body, status, created_at, updated_at`

// itemTables maps each thread item kind to the table its item_id references,
// used to validate that an attached item exists.
var itemTables = map[domain.ThreadItemKind]string{
	domain.ThreadItemIssue:      "issues",
	domain.ThreadItemDocument:   "documents",
	domain.ThreadItemTranscript: "transcripts",
	domain.ThreadItemComment:    "comments",
}

func (s *Store) migrateThreads() error {
	const schema = `
CREATE TABLE IF NOT EXISTS threads (
    id         TEXT PRIMARY KEY,
    project_id TEXT,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS thread_item (
    thread_id TEXT NOT NULL,
    kind      TEXT NOT NULL,
    item_id   TEXT NOT NULL,
    added_at  TEXT NOT NULL,
    UNIQUE(thread_id, kind, item_id)
);
CREATE INDEX IF NOT EXISTS idx_thread_item_thread ON thread_item (thread_id);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate threads: %w", err)
	}
	return nil
}

// CreateThread inserts a thread, validating its project reference (if any) in
// the same transaction so a concurrent project delete can't leave it dangling.
func (s *Store) CreateThread(t domain.Thread) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if t.ProjectID != nil {
		if err := existsInTx(tx, "projects", *t.ProjectID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO threads (`+threadColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, nullString(t.ProjectID), t.Title, t.Body, string(t.Status),
		t.CreatedAt.Format(timeFmt), t.UpdatedAt.Format(timeFmt),
	); err != nil {
		return fmt.Errorf("insert thread: %w", err)
	}
	return tx.Commit()
}

// GetThread returns a thread by id, or ErrNotFound.
func (s *Store) GetThread(id string) (domain.Thread, error) {
	return scanThread(s.db.QueryRow(`SELECT `+threadColumns+` FROM threads WHERE id = ?`, id))
}

// GetThreadView returns a thread and its members read in one transaction, so a
// concurrent delete cannot yield a thread with an impossible membership set.
// Returns ErrNotFound if the thread does not exist.
func (s *Store) GetThreadView(id string) (domain.Thread, []domain.ThreadItem, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Thread{}, nil, err
	}
	defer tx.Rollback()

	t, err := scanThread(tx.QueryRow(`SELECT `+threadColumns+` FROM threads WHERE id = ?`, id))
	if err != nil {
		return domain.Thread{}, nil, err
	}
	items, err := threadItems(tx, id)
	if err != nil {
		return domain.Thread{}, nil, err
	}
	return t, items, nil
}

// ListThreads returns threads ordered by creation. If projectID is non-empty it
// restricts to that project.
func (s *Store) ListThreads(projectID string) ([]domain.Thread, error) {
	q := `SELECT ` + threadColumns + ` FROM threads`
	var args []any
	if projectID != "" {
		q += ` WHERE project_id = ?`
		args = append(args, projectID)
	}
	q += ` ORDER BY created_at, id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	defer rows.Close()
	var out []domain.Thread
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateThreadTx reads a thread, applies mutate, stamps updated_at, validates,
// and writes — all in one transaction, so concurrent updates cannot clobber one
// another or regress updated_at. Returns ErrNotFound if the thread is missing.
func (s *Store) UpdateThreadTx(id string, mutate func(*domain.Thread) error) (domain.Thread, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Thread{}, err
	}
	defer tx.Rollback()

	t, err := scanThread(tx.QueryRow(`SELECT `+threadColumns+` FROM threads WHERE id = ?`, id))
	if err != nil {
		return domain.Thread{}, err // ErrNotFound when absent
	}
	if err := mutate(&t); err != nil {
		return domain.Thread{}, err
	}
	t.UpdatedAt = time.Now().UTC()
	if err := t.Validate(); err != nil {
		return domain.Thread{}, err
	}
	if _, err := tx.Exec(
		`UPDATE threads SET project_id = ?, title = ?, body = ?, status = ?, updated_at = ? WHERE id = ?`,
		nullString(t.ProjectID), t.Title, t.Body, string(t.Status), t.UpdatedAt.Format(timeFmt), t.ID,
	); err != nil {
		return domain.Thread{}, fmt.Errorf("update thread: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Thread{}, err
	}
	return t, nil
}

// DeleteThread removes a thread and its memberships (thread_item rows). The
// referenced items themselves are untouched. Returns ErrNotFound if gone.
func (s *Store) DeleteThread(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM thread_item WHERE thread_id = ?`, id); err != nil {
		return fmt.Errorf("delete thread items: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM threads WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete thread: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit()
}

// AddThreadItem attaches an item to a thread, validating (in one transaction)
// that both the thread and the referenced item exist. It is idempotent: adding
// the same item again is a no-op. Returns ErrNotFound if either is missing.
func (s *Store) AddThreadItem(threadID string, kind domain.ThreadItemKind, itemID string) error {
	table, ok := itemTables[kind]
	if !ok {
		return errors.Join(domain.ErrInvalidThread, fmt.Errorf("invalid item kind %q", kind))
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := existsInTx(tx, "threads", threadID); err != nil {
		return err
	}
	if err := existsInTx(tx, table, itemID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO thread_item (thread_id, kind, item_id, added_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(thread_id, kind, item_id) DO NOTHING`,
		threadID, string(kind), itemID, time.Now().UTC().Format(timeFmt),
	); err != nil {
		return fmt.Errorf("attach thread item: %w", err)
	}
	return tx.Commit()
}

// RemoveThreadItem detaches an item from a thread. It verifies the thread exists
// (ErrNotFound otherwise) in the same transaction, and is idempotent when the
// membership is simply absent.
func (s *Store) RemoveThreadItem(threadID string, kind domain.ThreadItemKind, itemID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := existsInTx(tx, "threads", threadID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM thread_item WHERE thread_id = ? AND kind = ? AND item_id = ?`,
		threadID, string(kind), itemID,
	); err != nil {
		return fmt.Errorf("detach thread item: %w", err)
	}
	return tx.Commit()
}

// ThreadItems returns a thread's members in the order they were added.
func (s *Store) ThreadItems(threadID string) ([]domain.ThreadItem, error) {
	return threadItems(s.db, threadID)
}

// threadItems reads a thread's members via the given querier (DB or tx) so it
// can participate in a consistent-snapshot transaction.
func threadItems(q querier, threadID string) ([]domain.ThreadItem, error) {
	rows, err := q.Query(
		`SELECT kind, item_id, added_at FROM thread_item WHERE thread_id = ? ORDER BY added_at, item_id`, threadID)
	if err != nil {
		return nil, fmt.Errorf("list thread items: %w", err)
	}
	defer rows.Close()
	var out []domain.ThreadItem
	for rows.Next() {
		var (
			it      domain.ThreadItem
			kind    string
			addedAt string
		)
		if err := rows.Scan(&kind, &it.ItemID, &addedAt); err != nil {
			return nil, fmt.Errorf("scan thread item: %w", err)
		}
		it.Kind = domain.ThreadItemKind(kind)
		if it.AddedAt, err = time.Parse(timeFmt, addedAt); err != nil {
			return nil, fmt.Errorf("parse added_at: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// existsInTx returns ErrNotFound if no row with the given id exists in table.
func existsInTx(tx *sql.Tx, table, id string) error {
	var one int
	err := tx.QueryRow(`SELECT 1 FROM `+table+` WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check %s exists: %w", table, err)
	}
	return nil
}

func scanThread(row rowScanner) (domain.Thread, error) {
	var (
		t                    domain.Thread
		projectID            sql.NullString
		status               string
		createdAt, updatedAt string
	)
	err := row.Scan(&t.ID, &projectID, &t.Title, &t.Body, &status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Thread{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Thread{}, fmt.Errorf("scan thread: %w", err)
	}
	if projectID.Valid {
		t.ProjectID = &projectID.String
	}
	t.Status = domain.ThreadStatus(status)
	if t.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
		return domain.Thread{}, fmt.Errorf("parse created_at: %w", err)
	}
	if t.UpdatedAt, err = time.Parse(timeFmt, updatedAt); err != nil {
		return domain.Thread{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return t, nil
}
