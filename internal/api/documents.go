package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
)

type attachDocumentRequest struct {
	Kind    domain.DocumentKind `json:"kind"`
	Title   string              `json:"title"`
	Content string              `json:"content"`
}

func (s *Server) attachDocument(w http.ResponseWriter, r *http.Request) {
	var req attachDocumentRequest
	if !decodeBody(w, r, &req) {
		return
	}
	doc, err := s.svc.AttachDocument(r.PathValue("id"), req.Kind, req.Title, req.Content)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if errors.Is(err, domain.ErrInvalidDocument) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := s.svc.ListDocuments(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if docs == nil {
		docs = []domain.Document{}
	}
	writeJSON(w, http.StatusOK, docs)
}

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
	doc, err := s.svc.GetDocument(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

type updateDocumentRequest struct {
	Content string `json:"content"`
}

func (s *Server) updateDocument(w http.ResponseWriter, r *http.Request) {
	var req updateDocumentRequest
	if !decodeBody(w, r, &req) {
		return
	}
	doc, err := s.svc.UpdateDocument(r.PathValue("id"), req.Content)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if errors.Is(err, domain.ErrInvalidDocument) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) deleteDocument(w http.ResponseWriter, r *http.Request) {
	err := s.svc.RemoveDocument(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeBody decodes a single JSON object from the request body, rejecting
// malformed or trailing data. It writes an error response and returns false on
// failure. Mirrors the strict decoding used by createIssue.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "unexpected trailing data in body")
		return false
	}
	return true
}
