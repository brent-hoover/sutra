package api

import (
	"errors"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
)

type ingestTranscriptRequest struct {
	SourcePath string `json:"source_path"`
}

func (s *Server) ingestTranscript(w http.ResponseWriter, r *http.Request) {
	var req ingestTranscriptRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.SourcePath == "" {
		writeError(w, http.StatusBadRequest, "source_path is required")
		return
	}
	t, err := s.svc.IngestTranscript(req.SourcePath)
	if errors.Is(err, domain.ErrInvalidTranscript) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) getTranscript(w http.ResponseWriter, r *http.Request) {
	t, err := s.svc.GetTranscript(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "transcript not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type linkTranscriptRequest struct {
	IssueID string `json:"issue_id"`
}

func (s *Server) linkTranscript(w http.ResponseWriter, r *http.Request) {
	var req linkTranscriptRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.IssueID == "" {
		writeError(w, http.StatusBadRequest, "issue_id is required")
		return
	}
	t, err := s.svc.LinkTranscript(r.PathValue("id"), req.IssueID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "transcript or issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) listIssueTranscripts(w http.ResponseWriter, r *http.Request) {
	transcripts, err := s.svc.TranscriptsForIssue(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if transcripts == nil {
		transcripts = []domain.Transcript{}
	}
	writeJSON(w, http.StatusOK, transcripts)
}

func (s *Server) discoverTranscripts(w http.ResponseWriter, r *http.Request) {
	found, err := s.svc.DiscoverTranscripts(r.URL.Query().Get("dir"))
	if errors.Is(err, domain.ErrInvalidTranscript) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if found == nil {
		found = []domain.DiscoveredTranscript{}
	}
	writeJSON(w, http.StatusOK, found)
}
