package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

const skillColumns = `id, name, slug, description, content, created_at, updated_at`

func (s *Store) migrateSkills() error {
	const schema = `
CREATE TABLE IF NOT EXISTS skills (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate skills: %w", err)
	}
	return nil
}

// CreateSkill inserts a skill. A duplicate slug is rejected as ErrInvalidSkill.
func (s *Store) CreateSkill(sk domain.Skill) error {
	_, err := s.db.Exec(
		`INSERT INTO skills (`+skillColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sk.ID, sk.Name, sk.Slug, sk.Description, sk.Content,
		sk.CreatedAt.Format(timeFmt), sk.UpdatedAt.Format(timeFmt),
	)
	if isUniqueViolation(err) {
		return errors.Join(domain.ErrInvalidSkill, fmt.Errorf("slug %q already exists", sk.Slug))
	}
	if err != nil {
		return fmt.Errorf("insert skill: %w", err)
	}
	return nil
}

// GetSkill returns a skill by id, or ErrNotFound.
func (s *Store) GetSkill(id string) (domain.Skill, error) {
	return scanSkill(s.db.QueryRow(`SELECT `+skillColumns+` FROM skills WHERE id = ?`, id))
}

// ListSkills returns all skills ordered by name.
func (s *Store) ListSkills() ([]domain.Skill, error) {
	rows, err := s.db.Query(`SELECT ` + skillColumns + ` FROM skills ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	var out []domain.Skill
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// UpdateSkillTx reads a skill, applies mutate, stamps updated_at, validates, and
// writes — all in one transaction, so concurrent updates cannot clobber one
// another or regress updated_at. A duplicate slug is ErrInvalidSkill; a missing
// id is ErrNotFound.
func (s *Store) UpdateSkillTx(id string, mutate func(*domain.Skill) error) (domain.Skill, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Skill{}, err
	}
	defer tx.Rollback()

	sk, err := scanSkill(tx.QueryRow(`SELECT `+skillColumns+` FROM skills WHERE id = ?`, id))
	if err != nil {
		return domain.Skill{}, err // ErrNotFound when absent
	}
	origSlug := sk.Slug
	if err := mutate(&sk); err != nil {
		return domain.Skill{}, err
	}
	sk.UpdatedAt = time.Now().UTC()
	if err := sk.Validate(); err != nil {
		return domain.Skill{}, err
	}
	if sk.Slug != origSlug {
		if err := domain.ValidateSlug(sk.Slug); err != nil {
			return domain.Skill{}, errors.Join(domain.ErrInvalidSkill, err)
		}
	}
	_, err = tx.Exec(
		`UPDATE skills SET name = ?, slug = ?, description = ?, content = ?, updated_at = ? WHERE id = ?`,
		sk.Name, sk.Slug, sk.Description, sk.Content, sk.UpdatedAt.Format(timeFmt), sk.ID,
	)
	if isUniqueViolation(err) {
		return domain.Skill{}, errors.Join(domain.ErrInvalidSkill, fmt.Errorf("slug %q already exists", sk.Slug))
	}
	if err != nil {
		return domain.Skill{}, fmt.Errorf("update skill: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Skill{}, err
	}
	return sk, nil
}

// DeleteSkill removes a skill. Returns ErrNotFound if it does not exist.
func (s *Store) DeleteSkill(id string) error {
	res, err := s.db.Exec(`DELETE FROM skills WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete skill: %w", err)
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

func scanSkill(row rowScanner) (domain.Skill, error) {
	var (
		sk                   domain.Skill
		createdAt, updatedAt string
	)
	err := row.Scan(&sk.ID, &sk.Name, &sk.Slug, &sk.Description, &sk.Content, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Skill{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Skill{}, fmt.Errorf("scan skill: %w", err)
	}
	if sk.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
		return domain.Skill{}, fmt.Errorf("parse created_at: %w", err)
	}
	if sk.UpdatedAt, err = time.Parse(timeFmt, updatedAt); err != nil {
		return domain.Skill{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return sk, nil
}
