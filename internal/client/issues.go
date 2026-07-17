package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/brent-hoover/sutra/internal/domain"
)

// IssueResult carries both the decoded issue and the raw JSON response body,
// so callers can render either the typed value or the exact server response.
type IssueResult struct {
	Issue domain.Issue
	Raw   json.RawMessage
}

// CreateIssue creates an issue via the daemon.
func (c *Client) CreateIssue(ctx context.Context, subject, body string) (IssueResult, error) {
	payload, err := json.Marshal(map[string]string{"subject": subject, "body": body})
	if err != nil {
		return IssueResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/issues", bytes.NewReader(payload))
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusCreated)
}

// GetIssue fetches an issue by id via the daemon.
func (c *Client) GetIssue(ctx context.Context, id string) (IssueResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/issues/"+url.PathEscape(id), nil)
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	endpoint, err := c.resolve(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// resolve joins the base URL and endpoint path safely, so a trailing slash on
// SUTRA_HOST does not produce "//issues" (which would trigger a redirect that
// downgrades POST to GET).
func (c *Client) resolve(path string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid host %q: %w", c.baseURL, err)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func (c *Client) doIssue(req *http.Request, want int) (IssueResult, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return IssueResult{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return IssueResult{}, err
	}
	if resp.StatusCode != want {
		return IssueResult{}, fmt.Errorf("daemon returned %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	var issue domain.Issue
	if err := json.Unmarshal(raw, &issue); err != nil {
		return IssueResult{}, err
	}
	return IssueResult{Issue: issue, Raw: raw}, nil
}
