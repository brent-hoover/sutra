package service

import (
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateSkill creates a reusable agent skill. The slug defaults to a slugified
// name and must be canonical (URL-safe).
func (s *Service) CreateSkill(name, slug, description, content string) (domain.Skill, error) {
	if slug == "" {
		slug = domain.Slugify(name)
	}
	now := time.Now().UTC()
	sk := domain.Skill{
		ID:          domain.NewID(),
		Name:        name,
		Slug:        slug,
		Description: description,
		Content:     content,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := sk.Validate(); err != nil {
		return domain.Skill{}, err
	}
	if err := domain.ValidateSlug(sk.Slug); err != nil {
		return domain.Skill{}, err
	}
	if err := s.store.CreateSkill(sk); err != nil {
		return domain.Skill{}, err
	}
	return sk, nil
}

// GetSkill returns a skill by id (ErrNotFound if missing).
func (s *Service) GetSkill(id string) (domain.Skill, error) {
	return s.store.GetSkill(id)
}

// ListSkills returns all skills.
func (s *Service) ListSkills() ([]domain.Skill, error) {
	return s.store.ListSkills()
}

// SkillUpdate carries the mutable fields of a skill; nil fields are unchanged.
type SkillUpdate struct {
	Name        *string
	Slug        *string
	Description *string
	Content     *string
}

// UpdateSkill applies the non-nil fields of upd. Returns ErrNotFound if the
// skill does not exist, or ErrInvalidSkill on invalid input.
func (s *Service) UpdateSkill(id string, upd SkillUpdate) (domain.Skill, error) {
	return s.store.UpdateSkillTx(id, func(sk *domain.Skill) error {
		if upd.Name != nil {
			sk.Name = *upd.Name
		}
		if upd.Slug != nil {
			sk.Slug = *upd.Slug
		}
		if upd.Description != nil {
			sk.Description = *upd.Description
		}
		if upd.Content != nil {
			sk.Content = *upd.Content
		}
		return nil
	})
}

// DeleteSkill removes a skill. Returns ErrNotFound if it does not exist.
func (s *Service) DeleteSkill(id string) error {
	return s.store.DeleteSkill(id)
}
