package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// AddLabel adds a label to an issue via the daemon.
func (c *Client) AddLabel(ctx context.Context, issueID, label string) (IssueResult, error) {
	payload, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		return IssueResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(issueID)+"/labels", bytes.NewReader(payload))
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}

// RemoveLabel removes a label from an issue via the daemon. The label travels
// as a query parameter so free-text values like "." or ".." survive routing.
func (c *Client) RemoveLabel(ctx context.Context, issueID, label string) (IssueResult, error) {
	q := url.Values{"label": {label}}
	req, err := c.newRequest(ctx, http.MethodDelete, "/issues/"+url.PathEscape(issueID)+"/labels?"+q.Encode(), nil)
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}
