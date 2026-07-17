package domain

import (
	"errors"
	"time"
)

// DocumentKind classifies a doc-driven-development document.
type DocumentKind string

const (
	DocProblem   DocumentKind = "problem"
	DocDesign    DocumentKind = "design"
	DocPlan      DocumentKind = "plan"
	DocScenarios DocumentKind = "scenarios"
)

// ErrInvalidDocument is returned when a document fails validation.
var ErrInvalidDocument = errors.New("invalid document")

// Document is a doc-driven-development artifact attached to an issue.
type Document struct {
	ID        string       `json:"id"`
	IssueID   string       `json:"issue_id"`
	Kind      DocumentKind `json:"kind"`
	Title     string       `json:"title,omitempty"`
	Content   string       `json:"content"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Validate enforces that content is non-empty and kind is one of the allowed
// values.
func (d Document) Validate() error {
	switch d.Kind {
	case DocProblem, DocDesign, DocPlan, DocScenarios:
	default:
		return errors.Join(ErrInvalidDocument, errors.New("kind must be one of problem|design|plan|scenarios"))
	}
	if d.Content == "" {
		return errors.Join(ErrInvalidDocument, errors.New("content is required"))
	}
	return nil
}
