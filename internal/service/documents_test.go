package service_test

import (
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

func issueUpdatedAt(t *testing.T, svc *service.Service, id string) time.Time {
	t.Helper()
	iss, err := svc.GetIssue(id)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	return iss.UpdatedAt
}

// A document create/update/remove is a change to the owning issue, so each must
// advance the issue's updated_at.
func TestDocumentMutationsBumpIssueUpdatedAt(t *testing.T) {
	svc := newService(t, t.TempDir())
	iss, err := svc.CreateIssue("Doc host", "body")
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	t0 := issueUpdatedAt(t, svc, iss.ID)
	doc, err := svc.AttachDocument(iss.ID, domain.DocProblem, "Title", "content")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	t1 := issueUpdatedAt(t, svc, iss.ID)
	if !t1.After(t0) {
		t.Errorf("attach did not advance issue updated_at: %s !> %s", t1, t0)
	}

	if _, err := svc.UpdateDocument(doc.ID, "revised"); err != nil {
		t.Fatalf("update: %v", err)
	}
	t2 := issueUpdatedAt(t, svc, iss.ID)
	if !t2.After(t1) {
		t.Errorf("update did not advance issue updated_at: %s !> %s", t2, t1)
	}

	if err := svc.RemoveDocument(doc.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	t3 := issueUpdatedAt(t, svc, iss.ID)
	if !t3.After(t2) {
		t.Errorf("remove did not advance issue updated_at: %s !> %s", t3, t2)
	}
}
