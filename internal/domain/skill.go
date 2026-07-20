package domain

import (
	"errors"
	"strings"
	"time"
)

// Skill is a reusable agent skill: metadata plus the SKILL.md content that
// `skill install` writes to a client's skills directory. Skills are global (not
// scoped to a project).
type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ErrInvalidSkill is returned when a skill fails validation.
var ErrInvalidSkill = errors.New("invalid skill")

// Validate enforces a name, a slug, and non-empty content (the SKILL.md body).
func (s Skill) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.Join(ErrInvalidSkill, errors.New("name is required"))
	}
	if s.Slug == "" {
		return errors.Join(ErrInvalidSkill, errors.New("slug is required"))
	}
	if strings.TrimSpace(s.Content) == "" {
		return errors.Join(ErrInvalidSkill, errors.New("content is required"))
	}
	return nil
}
