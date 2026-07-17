package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

// decodeBody strictly decodes a single JSON object from the request body,
// rejecting malformed input and any trailing data after the object.
func decodeBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing data in body")
	}
	return nil
}

type createIssueRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Server) createIssue(w http.ResponseWriter, r *http.Request) {
	var req createIssueRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

func (s *Server) listIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Reject invalid enum filters rather than silently returning an empty list.
	if v := q.Get("status"); v != "" && !domain.Status(v).Valid() {
		writeError(w, http.StatusBadRequest, "invalid status filter: "+v)
		return
	}
	if v := q.Get("type"); v != "" && !domain.IssueType(v).Valid() {
		writeError(w, http.StatusBadRequest, "invalid type filter: "+v)
		return
	}
	if v := q.Get("priority"); v != "" && !domain.Priority(v).Valid() {
		writeError(w, http.StatusBadRequest, "invalid priority filter: "+v)
		return
	}
	issues, err := s.svc.ListIssues(domain.IssueFilter{
		Status:   domain.Status(q.Get("status")),
		Type:     domain.IssueType(q.Get("type")),
		Priority: domain.Priority(q.Get("priority")),
		Owner:    q.Get("owner"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if issues == nil {
		issues = []domain.Issue{}
	}
	writeJSON(w, http.StatusOK, issues)
}

type updateIssueRequest struct {
	Type     *domain.IssueType `json:"type"`
	Status   *domain.Status    `json:"status"`
	Priority *domain.Priority  `json:"priority"`
	Owner    *string           `json:"owner"`
}

func (s *Server) updateIssue(w http.ResponseWriter, r *http.Request) {
	var req updateIssueRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	issue, err := s.svc.UpdateIssue(r.PathValue("id"), service.IssueUpdate{
		Type:     req.Type,
		Status:   req.Status,
		Priority: req.Priority,
		Owner:    req.Owner,
	})
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

func (s *Server) deleteIssue(w http.ResponseWriter, r *http.Request) {
	issue, err := s.svc.SoftDeleteIssue(r.PathValue("id"))
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

func (s *Server) issueHistory(w http.ResponseWriter, r *http.Request) {
	entries, err := s.svc.IssueHistory(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []domain.LedgerEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
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
