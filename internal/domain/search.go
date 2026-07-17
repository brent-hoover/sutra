package domain

import "errors"

// ErrInvalidSearch is returned when a search request is malformed (empty query
// text or an unknown kind scope).
var ErrInvalidSearch = errors.New("invalid search")

// SearchKind identifies the source entity a search hit came from.
type SearchKind string

const (
	KindIssue    SearchKind = "issue"
	KindDocument SearchKind = "document"
	KindMessage  SearchKind = "message"
)

// Valid reports whether k is a known search kind.
func (k SearchKind) Valid() bool {
	switch k {
	case KindIssue, KindDocument, KindMessage:
		return true
	}
	return false
}

// SearchQuery scopes a full-text search. Text is the FTS query; Kind restricts
// to one source entity ("" = all); Issue restricts to a single issue's content
// ("" = all issues); Limit bounds the number of hits (0 = the store's default
// cap, which also caps any larger request).
type SearchQuery struct {
	Text  string
	Kind  SearchKind
	Issue string
	Limit int
}

// SearchHit is one ranked result. Exactly one of Issue/Document/Message is set,
// per Kind. Rank is the FTS5 bm25 score — lower is more relevant, so hits are
// ordered ascending.
//
// A message hit additionally carries its owning Transcript (without its full
// message list), the linked Issue if the transcript is linked, and a Context
// window of adjacent messages (for agent synthesis).
type SearchHit struct {
	Kind SearchKind `json:"kind"`
	Rank float64    `json:"rank"`

	Issue    *Issue    `json:"issue,omitempty"`
	Document *Document `json:"document,omitempty"`

	Message     *Message    `json:"message,omitempty"`
	Transcript  *Transcript `json:"transcript,omitempty"`
	LinkedIssue *Issue      `json:"linked_issue,omitempty"`
	Context     []Message   `json:"context,omitempty"`
}

// SearchResults is the structured response for a search.
type SearchResults struct {
	Query string      `json:"query"`
	Hits  []SearchHit `json:"hits"`
}
