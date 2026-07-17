package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// SetParent sets an issue's parent via the daemon.
func (c *Client) SetParent(ctx context.Context, childID, parentID string) (IssueResult, error) {
	payload, err := json.Marshal(map[string]string{"parent_id": parentID})
	if err != nil {
		return IssueResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPut, "/issues/"+url.PathEscape(childID)+"/parent", bytes.NewReader(payload))
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}

// RelateIssue records a symmetric related link between two issues via the daemon.
func (c *Client) RelateIssue(ctx context.Context, issueID, relatedID string) (IssueResult, error) {
	payload, err := json.Marshal(map[string]string{"related_issue_id": relatedID})
	if err != nil {
		return IssueResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(issueID)+"/relations", bytes.NewReader(payload))
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}

// BlockIssue records that blockerID blocks issueID via the daemon.
func (c *Client) BlockIssue(ctx context.Context, issueID, blockerID string) (IssueResult, error) {
	payload, err := json.Marshal(map[string]string{"blocker_id": blockerID})
	if err != nil {
		return IssueResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(issueID)+"/blocks", bytes.NewReader(payload))
	if err != nil {
		return IssueResult{}, err
	}
	return c.doIssue(req, http.StatusOK)
}
