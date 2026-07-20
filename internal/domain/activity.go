package domain

import "time"

// ActivityEventType classifies an entry in the activity feed.
type ActivityEventType string

const (
	// ActivityLedger is an issue change; see LedgerKind for the specific change.
	ActivityLedger ActivityEventType = "ledger"
	// ActivityTranscript is a captured Claude session.
	ActivityTranscript ActivityEventType = "transcript"
)

// ActivityEvent is one entry in the reverse-chronological activity feed. It
// unifies issue changes (from the ledger) and captured transcripts so a single
// feed answers "what was I working on".
type ActivityEvent struct {
	At   time.Time         `json:"at"`
	Type ActivityEventType `json:"type"`

	// Issue context: set for every ledger event, and for a transcript event when
	// the transcript is linked to an issue.
	IssueID      string `json:"issue_id,omitempty"`
	IssueSubject string `json:"issue_subject,omitempty"`

	// Ledger detail (Type == ActivityLedger).
	LedgerKind LedgerKind `json:"ledger_kind,omitempty"`
	Field      string     `json:"field,omitempty"`
	OldValue   string     `json:"old_value,omitempty"`
	NewValue   string     `json:"new_value,omitempty"`

	// Transcript detail (Type == ActivityTranscript).
	TranscriptID string `json:"transcript_id,omitempty"`
	Title        string `json:"title,omitempty"`
}

// ActivityFeed is the result of an activity query: the window start and the
// events within it, ordered newest-first.
type ActivityFeed struct {
	Since  time.Time       `json:"since"`
	Events []ActivityEvent `json:"events"`
}
