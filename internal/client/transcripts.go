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

// TranscriptResult carries a decoded transcript and the raw JSON response.
type TranscriptResult struct {
	Transcript domain.Transcript
	Raw        json.RawMessage
}

// TranscriptListResult carries a decoded transcript list and the raw response.
type TranscriptListResult struct {
	Transcripts []domain.Transcript
	Raw         json.RawMessage
}

// DiscoverResult carries discovered session files and the raw response.
type DiscoverResult struct {
	Found []domain.DiscoveredTranscript
	Raw   json.RawMessage
}

// IngestTranscript ingests a .jsonl session file via the daemon.
func (c *Client) IngestTranscript(ctx context.Context, sourcePath string) (TranscriptResult, error) {
	payload, err := json.Marshal(map[string]string{"source_path": sourcePath})
	if err != nil {
		return TranscriptResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/transcripts", bytes.NewReader(payload))
	if err != nil {
		return TranscriptResult{}, err
	}
	return c.doTranscript(req, http.StatusCreated)
}

// GetTranscript reads a transcript and its messages via the daemon.
func (c *Client) GetTranscript(ctx context.Context, id string) (TranscriptResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/transcripts/"+url.PathEscape(id), nil)
	if err != nil {
		return TranscriptResult{}, err
	}
	return c.doTranscript(req, http.StatusOK)
}

// LinkTranscript links a transcript to an issue via the daemon.
func (c *Client) LinkTranscript(ctx context.Context, transcriptID, issueID string) (TranscriptResult, error) {
	payload, err := json.Marshal(map[string]string{"issue_id": issueID})
	if err != nil {
		return TranscriptResult{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/transcripts/"+url.PathEscape(transcriptID)+"/link", bytes.NewReader(payload))
	if err != nil {
		return TranscriptResult{}, err
	}
	return c.doTranscript(req, http.StatusOK)
}

// TranscriptsForIssue lists the transcripts linked to an issue via the daemon.
func (c *Client) TranscriptsForIssue(ctx context.Context, issueID string) (TranscriptListResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/issues/"+url.PathEscape(issueID)+"/transcripts", nil)
	if err != nil {
		return TranscriptListResult{}, err
	}
	resp, raw, err := c.send(req, http.StatusOK)
	if err != nil {
		return TranscriptListResult{}, err
	}
	_ = resp
	var list []domain.Transcript
	if err := json.Unmarshal(raw, &list); err != nil {
		return TranscriptListResult{}, err
	}
	return TranscriptListResult{Transcripts: list, Raw: raw}, nil
}

// DiscoverTranscripts lists local session files via the daemon. An empty dir
// scans the default ~/.claude/projects location on the daemon host.
func (c *Client) DiscoverTranscripts(ctx context.Context, dir string) (DiscoverResult, error) {
	path := "/transcripts"
	if dir != "" {
		path += "?dir=" + url.QueryEscape(dir)
	}
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return DiscoverResult{}, err
	}
	_, raw, err := c.send(req, http.StatusOK)
	if err != nil {
		return DiscoverResult{}, err
	}
	var found []domain.DiscoveredTranscript
	if err := json.Unmarshal(raw, &found); err != nil {
		return DiscoverResult{}, err
	}
	return DiscoverResult{Found: found, Raw: raw}, nil
}

func (c *Client) doTranscript(req *http.Request, want int) (TranscriptResult, error) {
	_, raw, err := c.send(req, want)
	if err != nil {
		return TranscriptResult{}, err
	}
	var t domain.Transcript
	if err := json.Unmarshal(raw, &t); err != nil {
		return TranscriptResult{}, err
	}
	return TranscriptResult{Transcript: t, Raw: raw}, nil
}

// send performs the request and returns the raw body, erroring on any status
// other than want.
func (c *Client) send(req *http.Request, want int) (*http.Response, json.RawMessage, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != want {
		return nil, nil, fmt.Errorf("daemon returned %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	return resp, raw, nil
}
