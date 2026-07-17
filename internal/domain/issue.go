package domain

import (
	"crypto/rand"
	"errors"
	"time"
)

// IssueType classifies an issue.
type IssueType string

const (
	TypeFeature IssueType = "feature"
	TypeBug     IssueType = "bug"
	TypeTask    IssueType = "task"
	TypeChore   IssueType = "chore"
)

// Status is an issue's workflow state.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
)

// Priority ranks an issue from p0 (highest) to p3 (lowest).
type Priority string

const (
	P0 Priority = "p0"
	P1 Priority = "p1"
	P2 Priority = "p2"
	P3 Priority = "p3"
)

// ErrInvalidIssue is returned when an issue fails validation.
var ErrInvalidIssue = errors.New("invalid issue")

// Issue is the base unit of work.
type Issue struct {
	ID        string     `json:"id"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	Type      IssueType  `json:"type"`
	Status    Status     `json:"status"`
	Priority  Priority   `json:"priority"`
	Owner     string     `json:"owner,omitempty"`
	ParentID  *string    `json:"parent_id,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Validate enforces the invariant that subject and body are non-empty.
func (i Issue) Validate() error {
	if i.Subject == "" {
		return errors.Join(ErrInvalidIssue, errors.New("subject is required"))
	}
	if i.Body == "" {
		return errors.Join(ErrInvalidIssue, errors.New("body is required"))
	}
	return nil
}

const idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// NewID returns a short, stable, random identifier.
func NewID() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return string(b)
}
