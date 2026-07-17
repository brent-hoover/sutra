package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/brent-hoover/sutra/internal/domain"
)

// Search runs a full-text query across issues, documents, and transcript
// messages and returns ranked hits. Soft-deleted issues (and their documents,
// and messages of transcripts linked to them) are excluded — see store.Search.
//
// It validates the request: the query text must be non-empty and any kind scope
// must be a known kind, both returning ErrInvalidSearch.
func (s *Service) Search(q domain.SearchQuery) (domain.SearchResults, error) {
	if strings.TrimSpace(q.Text) == "" {
		return domain.SearchResults{}, errors.Join(domain.ErrInvalidSearch, errors.New("query text is required"))
	}
	if q.Kind != "" && !q.Kind.Valid() {
		return domain.SearchResults{}, errors.Join(domain.ErrInvalidSearch, fmt.Errorf("invalid kind %q", q.Kind))
	}
	hits, err := s.store.Search(q)
	if err != nil {
		return domain.SearchResults{}, err
	}
	if hits == nil {
		hits = []domain.SearchHit{}
	}
	return domain.SearchResults{Query: q.Text, Hits: hits}, nil
}
