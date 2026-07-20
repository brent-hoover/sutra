package api

import (
	"errors"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

type createThreadRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ProjectID string `json:"project_id"`
}

func (s *Server) createThread(w http.ResponseWriter, r *http.Request) {
	var req createThreadRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.svc.CreateThread(req.Title, req.Body, req.ProjectID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "project not found")
		return
	}
	if errors.Is(err, domain.ErrInvalidThread) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) listThreads(w http.ResponseWriter, r *http.Request) {
	threads, err := s.svc.ListThreads(r.URL.Query().Get("project"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if threads == nil {
		threads = []domain.Thread{}
	}
	writeJSON(w, http.StatusOK, threads)
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
	view, err := s.svc.GetThreadView(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type updateThreadRequest struct {
	Title  *string              `json:"title"`
	Body   *string              `json:"body"`
	Status *domain.ThreadStatus `json:"status"`
}

func (s *Server) updateThread(w http.ResponseWriter, r *http.Request) {
	var req updateThreadRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.svc.UpdateThread(r.PathValue("id"), service.ThreadUpdate{
		Title:  req.Title,
		Body:   req.Body,
		Status: req.Status,
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if errors.Is(err, domain.ErrInvalidThread) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteThread(w http.ResponseWriter, r *http.Request) {
	err := s.svc.DeleteThread(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type threadItemRequest struct {
	Kind   string `json:"kind"`
	ItemID string `json:"item_id"`
}

func (s *Server) addThreadItem(w http.ResponseWriter, r *http.Request) {
	var req threadItemRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := s.svc.AddThreadItem(r.PathValue("id"), domain.ThreadItemKind(req.Kind), req.ItemID)
	if errors.Is(err, domain.ErrInvalidThread) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "thread or item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return the updated view so the caller sees the current membership.
	view, err := s.svc.GetThreadView(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) removeThreadItem(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	err := s.svc.RemoveThreadItem(r.PathValue("id"), domain.ThreadItemKind(q.Get("kind")), q.Get("item_id"))
	if errors.Is(err, domain.ErrInvalidThread) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
