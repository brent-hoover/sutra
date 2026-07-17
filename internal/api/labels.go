package api

import (
	"net/http"
)

type addLabelRequest struct {
	Label string `json:"label"`
}

func (s *Server) addLabel(w http.ResponseWriter, r *http.Request) {
	var req addLabelRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	issue, err := s.svc.AddLabel(r.PathValue("id"), req.Label)
	writeLinkResult(w, issue, err)
}

func (s *Server) removeLabel(w http.ResponseWriter, r *http.Request) {
	issue, err := s.svc.RemoveLabel(r.PathValue("id"), r.PathValue("label"))
	writeLinkResult(w, issue, err)
}
