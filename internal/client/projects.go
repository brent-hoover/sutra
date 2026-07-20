package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/brent-hoover/sutra/internal/domain"
)

// ProjectResult carries a decoded project and the raw JSON response.
type ProjectResult struct {
	Project domain.Project
	Raw     json.RawMessage
}

// ProjectListResult carries decoded projects and the raw JSON response.
type ProjectListResult struct {
	Projects []domain.Project
	Raw      json.RawMessage
}

func (c *Client) doProject(req *http.Request, want int) (ProjectResult, error) {
	raw, err := c.do(req, want)
	if err != nil {
		return ProjectResult{}, err
	}
	var p domain.Project
	if err := json.Unmarshal(raw, &p); err != nil {
		return ProjectResult{}, err
	}
	return ProjectResult{Project: p, Raw: raw}, nil
}

// CreateProject creates a project via the daemon. Empty slug/description are
// omitted so the daemon applies its defaults.
func (c *Client) CreateProject(ctx context.Context, name, repoPath, slug, description string) (ProjectResult, error) {
	body := map[string]string{"name": name, "repo_path": repoPath}
	if slug != "" {
		body["slug"] = slug
	}
	if description != "" {
		body["description"] = description
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ProjectResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/projects", bytes.NewReader(payload))
	if err != nil {
		return ProjectResult{}, err
	}
	return c.doProject(req, http.StatusCreated)
}

// ListProjects lists all projects.
func (c *Client) ListProjects(ctx context.Context) (ProjectListResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/projects", nil)
	if err != nil {
		return ProjectListResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return ProjectListResult{}, err
	}
	var projects []domain.Project
	if err := json.Unmarshal(raw, &projects); err != nil {
		return ProjectListResult{}, err
	}
	return ProjectListResult{Projects: projects, Raw: raw}, nil
}

// GetProject fetches a project by id.
func (c *Client) GetProject(ctx context.Context, id string) (ProjectResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/projects/"+url.PathEscape(id), nil)
	if err != nil {
		return ProjectResult{}, err
	}
	return c.doProject(req, http.StatusOK)
}

// UpdateProject changes the given fields on a project. Only keys present in
// fields are sent.
func (c *Client) UpdateProject(ctx context.Context, id string, fields map[string]string) (ProjectResult, error) {
	payload, err := json.Marshal(fields)
	if err != nil {
		return ProjectResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPatch, "/projects/"+url.PathEscape(id), bytes.NewReader(payload))
	if err != nil {
		return ProjectResult{}, err
	}
	return c.doProject(req, http.StatusOK)
}

// DeleteProject deletes a project (detaching its references) via the daemon.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/projects/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, http.StatusNoContent)
	return err
}
