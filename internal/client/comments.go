package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/brent-hoover/sutra/internal/domain"
)

// CommentResult carries the decoded comment and the raw JSON response body.
type CommentResult struct {
	Comment domain.Comment
	Raw     json.RawMessage
}

// CommentListResult carries decoded comments and the raw JSON response body.
type CommentListResult struct {
	Comments []domain.Comment
	Raw      json.RawMessage
}

// ListComments fetches an issue's comments via the daemon.
func (c *Client) ListComments(ctx context.Context, issueID string) (CommentListResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/issues/"+url.PathEscape(issueID)+"/comments", nil)
	if err != nil {
		return CommentListResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return CommentListResult{}, err
	}
	var comments []domain.Comment
	if err := json.Unmarshal(raw, &comments); err != nil {
		return CommentListResult{}, err
	}
	return CommentListResult{Comments: comments, Raw: raw}, nil
}

// AddComment posts a comment to an issue via the daemon.
func (c *Client) AddComment(ctx context.Context, issueID, author, body string) (CommentResult, error) {
	payload, err := json.Marshal(map[string]string{"author": author, "body": body})
	if err != nil {
		return CommentResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(issueID)+"/comments", bytes.NewReader(payload))
	if err != nil {
		return CommentResult{}, err
	}
	raw, err := c.do(req, http.StatusCreated)
	if err != nil {
		return CommentResult{}, err
	}
	var comment domain.Comment
	if err := json.Unmarshal(raw, &comment); err != nil {
		return CommentResult{}, err
	}
	return CommentResult{Comment: comment, Raw: raw}, nil
}
