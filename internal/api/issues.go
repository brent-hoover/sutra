package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
)

type createIssueRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Server) createIssue(w http.ResponseWriter, r *http.Request) {
	var req createIssueRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "unexpected trailing data in body")
		return
	}
	issue, err := s.svc.CreateIssue(req.Subject, req.Body)
	if errors.Is(err, domain.ErrInvalidIssue) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, issue)
}

func (s *Server) getIssue(w http.ResponseWriter, r *http.Request) {
	issue, err := s.svc.GetIssue(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
