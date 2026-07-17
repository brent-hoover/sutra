package domain

import "time"

// LedgerKind names the kind of change recorded in the ledger.
type LedgerKind string

const (
	LedgerCreated       LedgerKind = "created"
	LedgerUpdated       LedgerKind = "updated"
	LedgerCommented     LedgerKind = "commented"
	LedgerStatusChanged LedgerKind = "status_changed"
	LedgerLinked        LedgerKind = "linked"
	LedgerDeleted       LedgerKind = "deleted"
)

// LedgerEntry is one append-only record of a change to an issue.
type LedgerEntry struct {
	ID       string     `json:"id"`
	IssueID  string     `json:"issue_id"`
	At       time.Time  `json:"at"`
	Actor    string     `json:"actor,omitempty"`
	Kind     LedgerKind `json:"kind"`
	Field    string     `json:"field,omitempty"`
	OldValue string     `json:"old_value,omitempty"`
	NewValue string     `json:"new_value,omitempty"`
}
