package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/brent-hoover/sutra/internal/domain"
)

// SkillResult carries a decoded skill and the raw JSON response.
type SkillResult struct {
	Skill domain.Skill
	Raw   json.RawMessage
}

// SkillListResult carries decoded skills and the raw JSON response.
type SkillListResult struct {
	Skills []domain.Skill
	Raw    json.RawMessage
}

func (c *Client) doSkill(req *http.Request, want int) (SkillResult, error) {
	raw, err := c.do(req, want)
	if err != nil {
		return SkillResult{}, err
	}
	var sk domain.Skill
	if err := json.Unmarshal(raw, &sk); err != nil {
		return SkillResult{}, err
	}
	return SkillResult{Skill: sk, Raw: raw}, nil
}

// CreateSkill creates a skill. Empty slug/description are omitted so the daemon
// applies its defaults.
func (c *Client) CreateSkill(ctx context.Context, name, slug, description, content string) (SkillResult, error) {
	body := map[string]string{"name": name, "content": content}
	if slug != "" {
		body["slug"] = slug
	}
	if description != "" {
		body["description"] = description
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return SkillResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/skills", bytes.NewReader(payload))
	if err != nil {
		return SkillResult{}, err
	}
	return c.doSkill(req, http.StatusCreated)
}

// ListSkills lists all skills.
func (c *Client) ListSkills(ctx context.Context) (SkillListResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/skills", nil)
	if err != nil {
		return SkillListResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return SkillListResult{}, err
	}
	var skills []domain.Skill
	if err := json.Unmarshal(raw, &skills); err != nil {
		return SkillListResult{}, err
	}
	return SkillListResult{Skills: skills, Raw: raw}, nil
}

// GetSkill fetches a skill by id.
func (c *Client) GetSkill(ctx context.Context, id string) (SkillResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/skills/"+url.PathEscape(id), nil)
	if err != nil {
		return SkillResult{}, err
	}
	return c.doSkill(req, http.StatusOK)
}

// UpdateSkill changes the given fields on a skill. Only keys present in fields
// are sent.
func (c *Client) UpdateSkill(ctx context.Context, id string, fields map[string]string) (SkillResult, error) {
	payload, err := json.Marshal(fields)
	if err != nil {
		return SkillResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPatch, "/skills/"+url.PathEscape(id), bytes.NewReader(payload))
	if err != nil {
		return SkillResult{}, err
	}
	return c.doSkill(req, http.StatusOK)
}

// DeleteSkill deletes a skill via the daemon.
func (c *Client) DeleteSkill(ctx context.Context, id string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/skills/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, http.StatusNoContent)
	return err
}
