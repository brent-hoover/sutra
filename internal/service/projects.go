package service

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateProject creates a project for a repository. The slug defaults to a
// slugified name; repoPath is normalized to an absolute path (1 project = 1
// repo) so transcript auto-association can match a session's working directory.
func (s *Service) CreateProject(name, repoPath, slug, description string) (domain.Project, error) {
	if slug == "" {
		slug = domain.Slugify(name)
	}
	now := time.Now().UTC()
	p := domain.Project{
		ID:          domain.NewID(),
		Name:        name,
		Slug:        slug,
		RepoPath:    cleanRepoPath(repoPath),
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := p.Validate(); err != nil {
		return domain.Project{}, err
	}
	if err := s.store.CreateProject(p); err != nil {
		return domain.Project{}, err
	}
	return p, nil
}

// cleanRepoPath normalizes a repo path lexically without resolving it against
// the daemon's working directory: a blank or relative path stays as-is so
// Project.Validate rejects it (the caller must supply an absolute path).
func cleanRepoPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	return filepath.Clean(p)
}

// GetProject returns a project by id (ErrNotFound if missing).
func (s *Service) GetProject(id string) (domain.Project, error) {
	return s.store.GetProject(id)
}

// ListProjects returns all projects.
func (s *Service) ListProjects() ([]domain.Project, error) {
	return s.store.ListProjects()
}

// ProjectUpdate carries the mutable fields of a project; nil fields are left
// unchanged.
type ProjectUpdate struct {
	Name        *string
	Slug        *string
	RepoPath    *string
	Description *string
}

// UpdateProject applies the non-nil fields of upd to a project. RepoPath is
// normalized to absolute. Returns ErrNotFound if the project does not exist.
func (s *Service) UpdateProject(id string, upd ProjectUpdate) (domain.Project, error) {
	return s.store.UpdateProjectTx(id, func(p *domain.Project) error {
		if upd.Name != nil {
			p.Name = *upd.Name
		}
		if upd.Slug != nil {
			p.Slug = *upd.Slug
		}
		if upd.RepoPath != nil {
			p.RepoPath = cleanRepoPath(*upd.RepoPath)
		}
		if upd.Description != nil {
			p.Description = *upd.Description
		}
		return nil
	})
}

// DeleteProject removes a project and detaches its references. Returns
// ErrNotFound if the project does not exist.
func (s *Service) DeleteProject(id string) error {
	return s.store.DeleteProject(id)
}

// projectForEncodedCWD returns the id of the project whose repo path encodes to
// encodedCWD (see domain.EncodeCWD), or "" if none matches.
func (s *Service) projectForEncodedCWD(encodedCWD string) (string, error) {
	return s.store.ProjectIDByEncodedCWD(encodedCWD)
}
