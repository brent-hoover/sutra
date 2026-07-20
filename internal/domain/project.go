package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Project is a first-class container for work in one repository (1 project = 1
// repo). Issues and threads may be scoped to a project, and transcripts
// auto-associate to the project whose RepoPath matches the session's cwd.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	RepoPath    string    `json:"repo_path"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ErrInvalidProject is returned when a project fails validation.
var ErrInvalidProject = errors.New("invalid project")

// Validate enforces that name, slug, and repo_path are present.
func (p Project) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.Join(ErrInvalidProject, errors.New("name is required"))
	}
	if strings.TrimSpace(p.RepoPath) == "" {
		return errors.Join(ErrInvalidProject, errors.New("repo_path is required"))
	}
	// 1 project = 1 repo: the path is the identity/match key, so it must be an
	// absolute path supplied by the caller — never resolved server-side.
	if !filepath.IsAbs(p.RepoPath) {
		return errors.Join(ErrInvalidProject, fmt.Errorf("repo_path must be absolute: %q", p.RepoPath))
	}
	if p.Slug == "" {
		return errors.Join(ErrInvalidProject, errors.New("slug is required"))
	}
	return nil
}

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify derives a URL-safe slug from arbitrary text: lowercased, with runs of
// non-alphanumeric characters collapsed to single hyphens and trimmed.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlugRun.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// EncodeCWD encodes a filesystem path the way Claude names its projects
// directory (each path separator becomes a hyphen), so a project's RepoPath can
// be matched against a transcript's encoded working-directory folder.
func EncodeCWD(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}
