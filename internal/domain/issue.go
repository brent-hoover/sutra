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
	// TypePlan is a plan issue: the parent of a set of tracer children, built
	// from a plan. Its body holds the plan prose; it carries an Approval that
	// records human sign-off. Approval is advisory — it does not restrict listing
	// or mutating the tracer children; agents are expected to honor it.
	TypePlan IssueType = "plan"
)

// Approval is the sign-off state of a plan issue. It is empty for non-plan
// issues and moves one-way from Pending to Approved.
type Approval string

const (
	ApprovalPending  Approval = "pending"
	ApprovalApproved Approval = "approved"
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

// Valid reports whether t is a known issue type.
func (t IssueType) Valid() bool {
	switch t {
	case TypeFeature, TypeBug, TypeTask, TypeChore, TypePlan:
		return true
	}
	return false
}

// Valid reports whether a is a known approval state.
func (a Approval) Valid() bool {
	switch a {
	case ApprovalPending, ApprovalApproved:
		return true
	}
	return false
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusClosed:
		return true
	}
	return false
}

// Valid reports whether p is a known priority.
func (p Priority) Valid() bool {
	switch p {
	case P0, P1, P2, P3:
		return true
	}
	return false
}

// IssueFilter narrows a list query. A zero-value field imposes no constraint;
// filters combine with AND.
type IssueFilter struct {
	Status    Status
	Type      IssueType
	Priority  Priority
	Owner     string
	Label     string // free-text label; matches issues carrying it in issue_label
	ProjectID string // scope to a single project
	ParentID  string // scope to a single parent (an issue's children, e.g. a plan's tracers)
}

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
	ProjectID *string    `json:"project_id,omitempty"`
	Approval  Approval   `json:"approval,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// IssueView is the full read-only projection of an issue: its own fields and
// derived labels, plus its related/blocking links and comments. It is what
// `sutra view <id>` and GET /issues/{id} return. Related, BlockedBy, and
// IsBlocking are lists of issue ids; BlockedBy is the set of issues blocking
// this one (incoming issue_block edges) and IsBlocking the set it blocks
// (outgoing edges).
type IssueView struct {
	Issue
	Related    []string  `json:"related"`
	BlockedBy  []string  `json:"blocked_by"`
	IsBlocking []string  `json:"is_blocking"`
	Comments   []Comment `json:"comments"`
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
