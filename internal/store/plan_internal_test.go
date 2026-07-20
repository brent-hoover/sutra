package store

import (
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// A plain issue created without an approval reads back with approval "" — the
// new column round-trips and defaults empty for non-plan issues.
func TestApprovalColumnRoundTripsEmpty(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "plain", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(iss, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	got, err := s.GetIssue(iss.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if got.Approval != "" {
		t.Errorf("approval = %q, want empty", got.Approval)
	}
}
