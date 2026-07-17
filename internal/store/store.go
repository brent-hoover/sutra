package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Store is the SQLite-backed persistence layer.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies
// the schema migrations.
func Open(path string) (*Store, error) {
	// 0700 keeps the DB and its -wal/-shm sidecars unreadable by other local
	// users — otherwise they could read data straight from disk, bypassing the
	// daemon's authentication.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// Single-user local daemon: serialize on one connection to avoid lock
	// contention, and set pragmas after opening rather than encoding them into
	// the path. WAL persists in the file; busy_timeout is per-connection.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	// The DB file now exists (migrate wrote to it); restrict it to the owner.
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("restrict db perms: %w", err)
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS issues (
    id         TEXT PRIMARY KEY,
    subject    TEXT NOT NULL,
    body       TEXT NOT NULL,
    type       TEXT NOT NULL,
    status     TEXT NOT NULL,
    priority   TEXT NOT NULL,
    owner      TEXT NOT NULL DEFAULT '',
    parent_id  TEXT,
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS ledger (
    id        TEXT PRIMARY KEY,
    issue_id  TEXT NOT NULL,
    at        TEXT NOT NULL,
    actor     TEXT NOT NULL DEFAULT '',
    kind      TEXT NOT NULL,
    field     TEXT NOT NULL DEFAULT '',
    old_value TEXT NOT NULL DEFAULT '',
    new_value TEXT NOT NULL DEFAULT ''
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
