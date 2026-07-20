package api

import (
	"net/http"
	"time"
)

// activity handles GET /activity?since=<RFC3339> — the reverse-chronological
// feed of issue changes and captured transcripts within the window.
func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		writeError(w, http.StatusBadRequest, "since is required")
		return
	}
	// RFC3339Nano parses timestamps both with and without sub-second precision.
	since, err := time.Parse(time.RFC3339Nano, sinceStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid since (want RFC3339): "+sinceStr)
		return
	}
	feed, err := s.svc.Activity(since.UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, feed)
}
