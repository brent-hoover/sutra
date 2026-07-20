package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

const projectColumns = `id, name, slug, repo_path, description, created_at, updated_at`

func (s *Store) migrateProjects() error {
	const schema = `
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    repo_path   TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate projects: %w", err)
	}
	// Additive project_id columns for databases created before projects existed.
	if err := s.addColumnIfMissing("issues", "project_id", "TEXT"); err != nil {
		return fmt.Errorf("migrate issues project_id: %w", err)
	}
	if err := s.addColumnIfMissing("transcripts", "project_id", "TEXT"); err != nil {
		return fmt.Errorf("migrate transcripts project_id: %w", err)
	}
	return nil
}

// CreateProject inserts a project in one transaction, rejecting a duplicate
// slug/repo_path (UNIQUE) and a repo path whose encoded-cwd collides with an
// existing project's (EncodeCWD is not injective) — both as ErrInvalidProject.
func (s *Store) CreateProject(p domain.Project) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := ensureNoEncodedCollision(tx, p.RepoPath, ""); err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO projects (`+projectColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Slug, p.RepoPath, p.Description,
		p.CreatedAt.Format(timeFmt), p.UpdatedAt.Format(timeFmt),
	)
	if isUniqueViolation(err) {
		return errors.Join(domain.ErrInvalidProject, fmt.Errorf("slug %q or repo_path %q already exists", p.Slug, p.RepoPath))
	}
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return tx.Commit()
}

// ensureNoEncodedCollision returns ErrInvalidProject if another project (id !=
// excludeID) has a repo path that encodes to the same folder as repoPath, which
// would make transcript auto-association ambiguous.
func ensureNoEncodedCollision(tx *sql.Tx, repoPath, excludeID string) error {
	want := domain.EncodeCWD(repoPath)
	rows, err := tx.Query(`SELECT id, repo_path FROM projects`)
	if err != nil {
		return fmt.Errorf("check encoded collision: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, rp string
		if err := rows.Scan(&id, &rp); err != nil {
			return err
		}
		if id != excludeID && domain.EncodeCWD(rp) == want {
			return errors.Join(domain.ErrInvalidProject,
				fmt.Errorf("repo path encodes to the same folder as an existing project: %q", rp))
		}
	}
	return rows.Err()
}

// ProjectIDByEncodedCWD returns the id of the project whose repo path encodes to
// the same folder as encodedCWD, or "" if none. New projects can't collide
// (ensureNoEncodedCollision), but a database predating that check might; rather
// than pick one non-deterministically, an ambiguous match returns "" (no
// association) so ingest never guesses wrong.
func (s *Store) ProjectIDByEncodedCWD(encodedCWD string) (string, error) {
	return projectIDByEncodedCWD(s.db, encodedCWD)
}

// projectIDByEncodedCWD resolves a project via the given querier (DB or tx) so
// the match can happen inside a transcript's upsert transaction.
func projectIDByEncodedCWD(q querier, encodedCWD string) (string, error) {
	rows, err := q.Query(`SELECT id, repo_path FROM projects`)
	if err != nil {
		return "", fmt.Errorf("match project by cwd: %w", err)
	}
	defer rows.Close()
	var match string
	for rows.Next() {
		var id, rp string
		if err := rows.Scan(&id, &rp); err != nil {
			return "", err
		}
		if domain.EncodeCWD(rp) == encodedCWD {
			if match != "" {
				return "", nil // ambiguous (legacy collision): don't guess
			}
			match = id
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return match, nil
}

// GetProject returns a project by id, or ErrNotFound.
func (s *Store) GetProject(id string) (domain.Project, error) {
	return scanProject(s.db.QueryRow(`SELECT `+projectColumns+` FROM projects WHERE id = ?`, id))
}

// ListProjects returns all projects ordered by name.
func (s *Store) ListProjects() ([]domain.Project, error) {
	rows, err := s.db.Query(`SELECT ` + projectColumns + ` FROM projects ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProjectTx reads a project, applies mutate, stamps updated_at, validates,
// and writes — all in one transaction, so concurrent updates cannot clobber one
// another or regress updated_at. A duplicate/colliding slug or repo_path is
// ErrInvalidProject; a missing id is ErrNotFound.
func (s *Store) UpdateProjectTx(id string, mutate func(*domain.Project) error) (domain.Project, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Project{}, err
	}
	defer tx.Rollback()

	p, err := scanProject(tx.QueryRow(`SELECT `+projectColumns+` FROM projects WHERE id = ?`, id))
	if err != nil {
		return domain.Project{}, err // ErrNotFound when absent
	}
	origRepoPath := p.RepoPath
	if err := mutate(&p); err != nil {
		return domain.Project{}, err
	}
	p.UpdatedAt = time.Now().UTC()
	if err := p.Validate(); err != nil {
		return domain.Project{}, err
	}
	// Only re-check encoded collisions when the repo path actually changed, so a
	// legacy project with a colliding path can still update its other fields (and
	// can change its path to a non-colliding one).
	if p.RepoPath != origRepoPath {
		if err := ensureNoEncodedCollision(tx, p.RepoPath, p.ID); err != nil {
			return domain.Project{}, err
		}
	}
	_, err = tx.Exec(
		`UPDATE projects SET name = ?, slug = ?, repo_path = ?, description = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.Slug, p.RepoPath, p.Description, p.UpdatedAt.Format(timeFmt), p.ID,
	)
	if isUniqueViolation(err) {
		return domain.Project{}, errors.Join(domain.ErrInvalidProject, fmt.Errorf("slug %q or repo_path %q already exists", p.Slug, p.RepoPath))
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("update project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Project{}, err
	}
	return p, nil
}

// DeleteProject removes a project and detaches its references: issues, threads,
// and transcripts scoped to it have their project_id cleared (no cascade
// deletes), all in one transaction. Returns ErrNotFound if the project is gone.
func (s *Store) DeleteProject(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// Detach issues one by one so each detachment advances updated_at and appends
	// a ledger entry (the issue contract requires both for every change), keeping
	// the detach visible in history and the activity feed.
	issueIDs, err := scanIDs(tx, `SELECT id FROM issues WHERE project_id = ?`, id)
	if err != nil {
		return fmt.Errorf("find scoped issues: %w", err)
	}
	for _, iid := range issueIDs {
		if _, err := tx.Exec(`UPDATE issues SET project_id = NULL, updated_at = ? WHERE id = ?`, now.Format(timeFmt), iid); err != nil {
			return fmt.Errorf("detach issue: %w", err)
		}
		if err := insertLedger(tx, []domain.LedgerEntry{{
			ID: domain.NewID(), IssueID: iid, At: now, Kind: domain.LedgerUpdated,
			Field: "project_id", OldValue: id, NewValue: "",
		}}); err != nil {
			return err
		}
	}
	// Threads carry no ledger; advance their updated_at on detach.
	if _, err := tx.Exec(`UPDATE threads SET project_id = NULL, updated_at = ? WHERE project_id = ?`, now.Format(timeFmt), id); err != nil {
		return fmt.Errorf("detach threads: %w", err)
	}
	if _, err := tx.Exec(`UPDATE transcripts SET project_id = NULL WHERE project_id = ?`, id); err != nil {
		return fmt.Errorf("detach transcripts: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
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

// scanIDs runs a single-column id query and returns the ids.
func scanIDs(tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanProject(row rowScanner) (domain.Project, error) {
	var (
		p                    domain.Project
		createdAt, updatedAt string
	)
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.RepoPath, &p.Description, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("scan project: %w", err)
	}
	if p.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
		return domain.Project{}, fmt.Errorf("parse created_at: %w", err)
	}
	if p.UpdatedAt, err = time.Parse(timeFmt, updatedAt); err != nil {
		return domain.Project{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return p, nil
}
