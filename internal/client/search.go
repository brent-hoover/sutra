package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/brent-hoover/sutra/internal/domain"
)

// SearchResult carries the decoded search results and the raw JSON response.
type SearchResult struct {
	Results domain.SearchResults
	Raw     json.RawMessage
}

// Search runs a full-text search via the daemon. kind and issue are optional
// scopes; empty values are omitted.
func (c *Client) Search(ctx context.Context, text, kind, issue string) (SearchResult, error) {
	q := url.Values{}
	q.Set("q", text)
	if kind != "" {
		q.Set("kind", kind)
	}
	if issue != "" {
		q.Set("issue", issue)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/search?"+q.Encode(), nil)
	if err != nil {
		return SearchResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return SearchResult{}, err
	}
	var results domain.SearchResults
	if err := json.Unmarshal(raw, &results); err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Results: results, Raw: raw}, nil
}
