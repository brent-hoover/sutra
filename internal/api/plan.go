package api

import (
	"errors"
	"net/http"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

type planStepRequest struct {
	Subject  string           `json:"subject"`
	Body     string           `json:"body"`
	Type     domain.IssueType `json:"type"`
	Priority domain.Priority  `json:"priority"`
}

type buildPlanRequest struct {
	Title     string            `json:"title"`
	Prose     string            `json:"prose"`
	ParentID  *string           `json:"parent_id"`
	ProjectID *string           `json:"project_id"`
	Steps     []planStepRequest `json:"steps"`
}

type planResponse struct {
	Plan     domain.Issue   `json:"plan"`
	Children []domain.Issue `json:"children"`
}

// buildPlan handles POST /plans: build a plan issue + tracer children in one
// atomic call.
func (s *Server) buildPlan(w http.ResponseWriter, r *http.Request) {
	var req buildPlanRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	steps := make([]service.PlanStep, len(req.Steps))
	for i, st := range req.Steps {
		steps[i] = service.PlanStep{Subject: st.Subject, Body: st.Body, Type: st.Type, Priority: st.Priority}
	}
	plan, children, err := s.svc.BuildPlan(req.Title, req.Prose, steps, req.ParentID, req.ProjectID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "parent or project not found")
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
	if children == nil {
		children = []domain.Issue{}
	}
	writeJSON(w, http.StatusCreated, planResponse{Plan: plan, Children: children})
}

// approvePlan handles POST /issues/{id}/plan/approve: 404 if the issue is
// missing, 400 if it is not a plan issue, 200 with the approved issue otherwise.
func (s *Server) approvePlan(w http.ResponseWriter, r *http.Request) {
	issue, err := s.svc.ApprovePlan(r.PathValue("id"))
	writeLinkResult(w, issue, err)
}
