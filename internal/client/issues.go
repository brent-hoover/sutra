package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CreateIssue creates an issue via the daemon.
func (c *Client) CreateIssue(ctx context.Context, subject, body string) (domain.Issue, error) {
	payload, err := json.Marshal(map[string]string{"subject": subject, "body": body})
	if err != nil {
		return domain.Issue{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/issues", bytes.NewReader(payload))
	if err != nil {
		return domain.Issue{}, err
	}
	return c.doIssue(req, http.StatusCreated)
}

// GetIssue fetches an issue by id via the daemon.
func (c *Client) GetIssue(ctx context.Context, id string) (domain.Issue, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/issues/"+id, nil)
	if err != nil {
		return domain.Issue{}, err
	}
	return c.doIssue(req, http.StatusOK)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
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

func (c *Client) doIssue(req *http.Request, want int) (domain.Issue, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return domain.Issue{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		return domain.Issue{}, fmt.Errorf("daemon returned %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	var issue domain.Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return domain.Issue{}, err
	}
	return issue, nil
}
