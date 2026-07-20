package api

import (
	"errors"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

type createSkillRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	var req createSkillRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sk, err := s.svc.CreateSkill(req.Name, req.Slug, req.Description, req.Content)
	if errors.Is(err, domain.ErrInvalidSkill) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sk)
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.svc.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if skills == nil {
		skills = []domain.Skill{}
	}
	writeJSON(w, http.StatusOK, skills)
}

func (s *Server) getSkill(w http.ResponseWriter, r *http.Request) {
	sk, err := s.svc.GetSkill(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

type updateSkillRequest struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Description *string `json:"description"`
	Content     *string `json:"content"`
}

func (s *Server) updateSkill(w http.ResponseWriter, r *http.Request) {
	var req updateSkillRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sk, err := s.svc.UpdateSkill(r.PathValue("id"), service.SkillUpdate{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Content:     req.Content,
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if errors.Is(err, domain.ErrInvalidSkill) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	err := s.svc.DeleteSkill(r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
