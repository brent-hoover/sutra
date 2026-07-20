package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/brent-hoover/sutra/internal/domain"
)

// ThreadResult carries a decoded thread and the raw JSON response.
type ThreadResult struct {
	Thread domain.Thread
	Raw    json.RawMessage
}

// ThreadViewResult carries a decoded thread view (with members) and the raw JSON.
type ThreadViewResult struct {
	View domain.ThreadView
	Raw  json.RawMessage
}

// ThreadListResult carries decoded threads and the raw JSON response.
type ThreadListResult struct {
	Threads []domain.Thread
	Raw     json.RawMessage
}

func (c *Client) doThread(req *http.Request, want int) (ThreadResult, error) {
	raw, err := c.do(req, want)
	if err != nil {
		return ThreadResult{}, err
	}
	var t domain.Thread
	if err := json.Unmarshal(raw, &t); err != nil {
		return ThreadResult{}, err
	}
	return ThreadResult{Thread: t, Raw: raw}, nil
}

// CreateThread creates a thread. An empty projectID/body is omitted.
func (c *Client) CreateThread(ctx context.Context, title, body, projectID string) (ThreadResult, error) {
	payload := map[string]string{"title": title}
	if body != "" {
		payload["body"] = body
	}
	if projectID != "" {
		payload["project_id"] = projectID
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ThreadResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/threads", bytes.NewReader(b))
	if err != nil {
		return ThreadResult{}, err
	}
	return c.doThread(req, http.StatusCreated)
}

// ListThreads lists threads, optionally restricted to a project.
func (c *Client) ListThreads(ctx context.Context, projectID string) (ThreadListResult, error) {
	path := "/threads"
	if projectID != "" {
		q := url.Values{}
		q.Set("project", projectID)
		path += "?" + q.Encode()
	}
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return ThreadListResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return ThreadListResult{}, err
	}
	var threads []domain.Thread
	if err := json.Unmarshal(raw, &threads); err != nil {
		return ThreadListResult{}, err
	}
	return ThreadListResult{Threads: threads, Raw: raw}, nil
}

// GetThread fetches a thread with its members.
func (c *Client) GetThread(ctx context.Context, id string) (ThreadViewResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/threads/"+url.PathEscape(id), nil)
	if err != nil {
		return ThreadViewResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return ThreadViewResult{}, err
	}
	var view domain.ThreadView
	if err := json.Unmarshal(raw, &view); err != nil {
		return ThreadViewResult{}, err
	}
	return ThreadViewResult{View: view, Raw: raw}, nil
}

// UpdateThread changes the given fields on a thread.
func (c *Client) UpdateThread(ctx context.Context, id string, fields map[string]string) (ThreadResult, error) {
	payload, err := json.Marshal(fields)
	if err != nil {
		return ThreadResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPatch, "/threads/"+url.PathEscape(id), bytes.NewReader(payload))
	if err != nil {
		return ThreadResult{}, err
	}
	return c.doThread(req, http.StatusOK)
}

// DeleteThread deletes a thread (its members are untouched).
func (c *Client) DeleteThread(ctx context.Context, id string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/threads/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, http.StatusNoContent)
	return err
}

// AddThreadItem attaches an item to a thread and returns the updated view.
func (c *Client) AddThreadItem(ctx context.Context, threadID, kind, itemID string) (ThreadViewResult, error) {
	payload, err := json.Marshal(map[string]string{"kind": kind, "item_id": itemID})
	if err != nil {
		return ThreadViewResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/threads/"+url.PathEscape(threadID)+"/items", bytes.NewReader(payload))
	if err != nil {
		return ThreadViewResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return ThreadViewResult{}, err
	}
	var view domain.ThreadView
	if err := json.Unmarshal(raw, &view); err != nil {
		return ThreadViewResult{}, err
	}
	return ThreadViewResult{View: view, Raw: raw}, nil
}

// RemoveThreadItem detaches an item from a thread.
func (c *Client) RemoveThreadItem(ctx context.Context, threadID, kind, itemID string) error {
	q := url.Values{}
	q.Set("kind", kind)
	q.Set("item_id", itemID)
	req, err := c.newRequest(ctx, http.MethodDelete, "/threads/"+url.PathEscape(threadID)+"/items?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, http.StatusNoContent)
	return err
}
