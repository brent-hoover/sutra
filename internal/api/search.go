package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/brent-hoover/sutra/internal/domain"
)

// search handles GET /search?q=...&kind=...&issue=... — full-text search across
// issues, documents, and transcript messages.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	text := q.Get("q")
	if text == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	kind := domain.SearchKind(q.Get("kind"))
	if kind != "" && !kind.Valid() {
		writeError(w, http.StatusBadRequest, "invalid kind scope: "+string(kind))
		return
	}
	var limit int
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit: "+v)
			return
		}
		limit = n
	}
	results, err := s.svc.Search(domain.SearchQuery{
		Text:  text,
		Kind:  kind,
		Issue: q.Get("issue"),
		Limit: limit,
	})
	if errors.Is(err, domain.ErrInvalidSearch) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}
