package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ThreadStatus is a thread's lifecycle state.
type ThreadStatus string

const (
	ThreadActive   ThreadStatus = "active"
	ThreadArchived ThreadStatus = "archived"
)

// Valid reports whether s is a known thread status.
func (s ThreadStatus) Valid() bool { return s == ThreadActive || s == ThreadArchived }

// ThreadItemKind names the type of entity a thread member points at.
type ThreadItemKind string

const (
	ThreadItemIssue      ThreadItemKind = "issue"
	ThreadItemDocument   ThreadItemKind = "document"
	ThreadItemTranscript ThreadItemKind = "transcript"
	ThreadItemComment    ThreadItemKind = "comment"
)

// Valid reports whether k is a known thread item kind.
func (k ThreadItemKind) Valid() bool {
	switch k {
	case ThreadItemIssue, ThreadItemDocument, ThreadItemTranscript, ThreadItemComment:
		return true
	}
	return false
}

// Thread is a meta-object tying together the pieces of one line of work
// (issues, documents, transcripts, comments). It may be scoped to a project.
type Thread struct {
	ID        string       `json:"id"`
	ProjectID *string      `json:"project_id,omitempty"`
	Title     string       `json:"title"`
	Body      string       `json:"body,omitempty"`
	Status    ThreadStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// ThreadItem is one membership: a typed reference to an entity in a thread.
type ThreadItem struct {
	Kind    ThreadItemKind `json:"kind"`
	ItemID  string         `json:"item_id"`
	AddedAt time.Time      `json:"added_at"`
}

// ThreadView is a thread with its members, for read/display.
type ThreadView struct {
	Thread
	Items []ThreadItem `json:"items"`
}

// ErrInvalidThread is returned when a thread fails validation.
var ErrInvalidThread = errors.New("invalid thread")

// Validate enforces a non-empty title and a valid status.
func (t Thread) Validate() error {
	if strings.TrimSpace(t.Title) == "" {
		return errors.Join(ErrInvalidThread, errors.New("title is required"))
	}
	if !t.Status.Valid() {
		return errors.Join(ErrInvalidThread, fmt.Errorf("invalid status %q", t.Status))
	}
	return nil
}
