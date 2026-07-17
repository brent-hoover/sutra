package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
)

type ingestTranscriptRequest struct {
	SourcePath string `json:"source_path"`
}

func (s *Server) ingestTranscript(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBody[ingestTranscriptRequest](w, r)
	if !ok {
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
	req, ok := decodeBody[linkTranscriptRequest](w, r)
	if !ok {
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
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if found == nil {
		found = []domain.DiscoveredTranscript{}
	}
	writeJSON(w, http.StatusOK, found)
}

// decodeBody decodes exactly one JSON object of type T from the request body,
// rejecting malformed or concatenated input — the same contract as the issue
// handler's inline decoding.
func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return v, false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "unexpected trailing data in body")
		return v, false
	}
	return v, true
}
