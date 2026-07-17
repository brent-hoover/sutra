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

// CreateChildIssue creates an issue whose parent_id is set to parentID, in a
// single request so the issue and its parent link are persisted atomically.
func (c *Client) CreateChildIssue(ctx context.Context, subject, body, parentID string) (IssueResult, error) {
	payload, err := json.Marshal(map[string]string{"subject": subject, "body": body, "parent_id": parentID})
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

// IssueListResult carries the decoded issues and the raw JSON response body.
type IssueListResult struct {
	Issues []domain.Issue
	Raw    json.RawMessage
}

// ListIssues lists issues, applying the given filters as query parameters.
// Empty filter values are omitted.
func (c *Client) ListIssues(ctx context.Context, filters map[string]string) (IssueListResult, error) {
	q := url.Values{}
	for k, v := range filters {
		if v != "" {
			q.Set(k, v)
		}
	}
	path := "/issues"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return IssueListResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return IssueListResult{}, err
	}
	var issues []domain.Issue
	if err := json.Unmarshal(raw, &issues); err != nil {
		return IssueListResult{}, err
	}
	return IssueListResult{Issues: issues, Raw: raw}, nil
}

// UpdateIssue changes the given fields on an issue. Only keys present in fields
// are sent, so callers change exactly the fields they mean to.
func (c *Client) UpdateIssue(ctx context.Context, id string, fields map[string]string) (IssueResult, error) {
	payload, err := json.Marshal(fields)
	if err != nil {
		return IssueResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPatch, "/issues/"+url.PathEscape(id), bytes.NewReader(payload))
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}

// DeleteIssue soft-deletes an issue via the daemon.
func (c *Client) DeleteIssue(ctx context.Context, id string) (IssueResult, error) {
	req, err := c.newRequest(ctx, http.MethodDelete, "/issues/"+url.PathEscape(id), nil)
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}

// HistoryResult carries an issue's ledger entries and the raw JSON response.
type HistoryResult struct {
	Entries []domain.LedgerEntry
	Raw     json.RawMessage
}

// History fetches an issue's change history via the daemon.
func (c *Client) History(ctx context.Context, id string) (HistoryResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/issues/"+url.PathEscape(id)+"/history", nil)
	if err != nil {
		return HistoryResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return HistoryResult{}, err
	}
	var entries []domain.LedgerEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return HistoryResult{}, err
	}
	return HistoryResult{Entries: entries, Raw: raw}, nil
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

// do executes a request, checks the status code, and returns the raw body.
func (c *Client) do(req *http.Request, want int) (json.RawMessage, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != want {
		return nil, fmt.Errorf("daemon returned %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	return raw, nil
}

func (c *Client) doIssue(req *http.Request, want int) (IssueResult, error) {
	raw, err := c.do(req, want)
	if err != nil {
		return IssueResult{}, err
	}
	var issue domain.Issue
	if err := json.Unmarshal(raw, &issue); err != nil {
		return IssueResult{}, err
	}
	return IssueResult{Issue: issue, Raw: raw}, nil
}
