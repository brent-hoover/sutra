package api

import (
	"errors"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
)

type createCommentRequest struct {
	Author string `json:"author"`
	Body   string `json:"body"`
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	comments, err := s.svc.IssueComments(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if comments == nil {
		comments = []domain.Comment{}
	}
	writeJSON(w, http.StatusOK, comments)
}

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	var req createCommentRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	comment, err := s.svc.CommentIssue(r.PathValue("id"), req.Author, req.Body)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if errors.Is(err, domain.ErrInvalidComment) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, comment)
}
