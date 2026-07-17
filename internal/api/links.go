package api

import (
	"errors"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
)

type setParentRequest struct {
	ParentID string `json:"parent_id"`
}

func (s *Server) setParent(w http.ResponseWriter, r *http.Request) {
	var req setParentRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	issue, err := s.svc.SetParent(r.PathValue("id"), req.ParentID)
	writeLinkResult(w, issue, err)
}

type relateRequest struct {
	RelatedIssueID string `json:"related_issue_id"`
}

func (s *Server) relateIssue(w http.ResponseWriter, r *http.Request) {
	var req relateRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	issue, err := s.svc.RelateIssues(r.PathValue("id"), req.RelatedIssueID)
	writeLinkResult(w, issue, err)
}

type blockRequest struct {
	BlockerID string `json:"blocker_id"`
}

func (s *Server) blockIssue(w http.ResponseWriter, r *http.Request) {
	var req blockRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The path issue is the blocked one; blocker_id blocks it.
	issue, err := s.svc.BlockIssue(r.PathValue("id"), req.BlockerID)
	writeLinkResult(w, issue, err)
}

// writeLinkResult maps a link mutation's outcome to an HTTP response: 404 for a
// missing issue, 400 for an invalid link (self/cycle), 200 with the issue else.
func writeLinkResult(w http.ResponseWriter, issue domain.Issue, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if errors.Is(err, domain.ErrInvalidIssue) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, issue)
}
