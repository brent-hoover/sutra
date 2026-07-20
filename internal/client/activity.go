package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// ActivityResult carries the decoded activity feed and the raw JSON response.
type ActivityResult struct {
	Feed domain.ActivityFeed
	Raw  json.RawMessage
}

// Activity fetches the activity feed at or after `since` from the daemon.
func (c *Client) Activity(ctx context.Context, since time.Time) (ActivityResult, error) {
	q := url.Values{}
	// RFC3339Nano preserves sub-second precision so the window boundary is not
	// rounded back by up to a second.
	q.Set("since", since.UTC().Format(time.RFC3339Nano))
	// POST: building the feed auto-ingests recent sessions (a persistent write),
	// so the endpoint is state-changing.
	req, err := c.newRequest(ctx, http.MethodPost, "/activity?"+q.Encode(), nil)
	if err != nil {
		return ActivityResult{}, err
	}
	raw, err := c.do(req, http.StatusOK)
	if err != nil {
		return ActivityResult{}, err
	}
	var feed domain.ActivityFeed
	if err := json.Unmarshal(raw, &feed); err != nil {
		return ActivityResult{}, err
	}
	return ActivityResult{Feed: feed, Raw: raw}, nil
}
