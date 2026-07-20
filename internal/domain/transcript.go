package domain

import (
	"errors"
	"time"
)

// Role classifies a transcript message's originator.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// ErrInvalidTranscript is returned when a transcript fails validation.
var ErrInvalidTranscript = errors.New("invalid transcript")

// Transcript is a captured Claude session — the container for its Messages.
// It is ingested from ~/.claude/projects/<encoded-cwd>/<session-uuid>.jsonl.
type Transcript struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	SourcePath string    `json:"source_path"`
	Title      string    `json:"title,omitempty"`
	IssueID    *string   `json:"issue_id,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
	CreatedAt  time.Time `json:"created_at"`
	Messages   []Message `json:"messages,omitempty"`
}

// Validate enforces that session_id and source_path are non-empty.
func (t Transcript) Validate() error {
	if t.SessionID == "" {
		return errors.Join(ErrInvalidTranscript, errors.New("session_id is required"))
	}
	if t.SourcePath == "" {
		return errors.Join(ErrInvalidTranscript, errors.New("source_path is required"))
	}
	return nil
}

// Message is one JSONL line within a Transcript.
type Message struct {
	ID           string     `json:"id"`
	TranscriptID string     `json:"transcript_id"`
	Seq          int        `json:"seq"`
	Role         Role       `json:"role"`
	Text         string     `json:"text,omitempty"`
	Raw          string     `json:"raw"`
	At           *time.Time `json:"at,omitempty"`
}

// DiscoveredTranscript is a Claude session file found on disk, with whether
// it has already been ingested. It is not persisted.
type DiscoveredTranscript struct {
	SessionID string    `json:"session_id"`
	Path      string    `json:"path"`
	Ingested  bool      `json:"ingested"`
	ModTime   time.Time `json:"mod_time"`
}
