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

// CreateProject inserts a project. A duplicate slug or repo_path is rejected as
// ErrInvalidProject (the UNIQUE constraints).
func (s *Store) CreateProject(p domain.Project) error {
	_, err := s.db.Exec(
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
	return nil
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

// UpdateProject writes name, slug, description, and repo_path (and updated_at).
// A duplicate slug/repo_path is ErrInvalidProject; a missing id is ErrNotFound.
func (s *Store) UpdateProject(p domain.Project) error {
	res, err := s.db.Exec(
		`UPDATE projects SET name = ?, slug = ?, repo_path = ?, description = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.Slug, p.RepoPath, p.Description, p.UpdatedAt.Format(timeFmt), p.ID,
	)
	if isUniqueViolation(err) {
		return errors.Join(domain.ErrInvalidProject, fmt.Errorf("slug %q or repo_path %q already exists", p.Slug, p.RepoPath))
	}
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
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

	for _, table := range []string{"issues", "threads", "transcripts"} {
		if _, err := tx.Exec(`UPDATE `+table+` SET project_id = NULL WHERE project_id = ?`, id); err != nil {
			return fmt.Errorf("detach %s: %w", table, err)
		}
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
