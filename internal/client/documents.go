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

// DocumentResult carries a decoded document and the raw JSON response body.
type DocumentResult struct {
	Document domain.Document
	Raw      json.RawMessage
}

// DocumentListResult carries decoded documents and the raw JSON response body.
type DocumentListResult struct {
	Documents []domain.Document
	Raw       json.RawMessage
}

// AttachDocument attaches a document to an issue via the daemon.
func (c *Client) AttachDocument(ctx context.Context, issueID, kind, title, content string) (DocumentResult, error) {
	payload, err := json.Marshal(map[string]string{"kind": kind, "title": title, "content": content})
	if err != nil {
		return DocumentResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(issueID)+"/documents", bytes.NewReader(payload))
	if err != nil {
		return DocumentResult{}, err
	}
	return c.doDocument(req, http.StatusCreated)
}

// GetDocument fetches a document by id via the daemon.
func (c *Client) GetDocument(ctx context.Context, id string) (DocumentResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/documents/"+url.PathEscape(id), nil)
	if err != nil {
		return DocumentResult{}, err
	}
	return c.doDocument(req, http.StatusOK)
}

// ListDocuments fetches an issue's documents via the daemon.
func (c *Client) ListDocuments(ctx context.Context, issueID string) (DocumentListResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/issues/"+url.PathEscape(issueID)+"/documents", nil)
	if err != nil {
		return DocumentListResult{}, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return DocumentListResult{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return DocumentListResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return DocumentListResult{}, fmt.Errorf("daemon returned %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	var docs []domain.Document
	if err := json.Unmarshal(raw, &docs); err != nil {
		return DocumentListResult{}, err
	}
	return DocumentListResult{Documents: docs, Raw: raw}, nil
}

// UpdateDocument updates a document's content via the daemon.
func (c *Client) UpdateDocument(ctx context.Context, id, content string) (DocumentResult, error) {
	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return DocumentResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPatch, "/documents/"+url.PathEscape(id), bytes.NewReader(payload))
	if err != nil {
		return DocumentResult{}, err
	}
	return c.doDocument(req, http.StatusOK)
}

// RemoveDocument deletes a document via the daemon, returning the raw JSON
// deletion result.
func (c *Client) RemoveDocument(ctx context.Context, id string) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, http.MethodDelete, "/documents/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	return raw, nil
}

func (c *Client) doDocument(req *http.Request, want int) (DocumentResult, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return DocumentResult{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return DocumentResult{}, err
	}
	if resp.StatusCode != want {
		return DocumentResult{}, fmt.Errorf("daemon returned %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	var doc domain.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return DocumentResult{}, err
	}
	return DocumentResult{Document: doc, Raw: raw}, nil
}
