package domain

import (
	"errors"
	"time"
)

// ErrInvalidComment is returned when a comment fails validation.
var ErrInvalidComment = errors.New("invalid comment")

// Comment is a note attached to an issue by a human or agent.
type Comment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issue_id"`
	Author    string    `json:"author,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate enforces that a comment names its issue and carries a body.
func (c Comment) Validate() error {
	if c.IssueID == "" {
		return errors.Join(ErrInvalidComment, errors.New("issue_id is required"))
	}
	if c.Body == "" {
		return errors.Join(ErrInvalidComment, errors.New("body is required"))
	}
	return nil
}
