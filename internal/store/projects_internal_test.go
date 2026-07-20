package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newProject(t *testing.T, s *Store, slug, repo string) domain.Project {
	t.Helper()
	now := time.Now().UTC()
	p := domain.Project{
		ID: domain.NewID(), Name: slug, Slug: slug, RepoPath: repo,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateProject(p); err != nil {
		t.Fatalf("create project %s: %v", slug, err)
	}
	return p
}

// Finding: deleting a project must audit each detached issue (updated_at + a
// project_id ledger entry), not silently clear the column.
func TestDeleteProjectDetachesIssueWithLedger(t *testing.T) {
	s := openStore(t)
	p := newProject(t, s, "alpha", "/repos/alpha")

	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "scoped", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		ProjectID: &p.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(iss, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	got, err := s.GetIssue(iss.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if got.ProjectID != nil {
		t.Errorf("project_id not cleared: %v", *got.ProjectID)
	}
	entries, err := s.LedgerFor(iss.ID)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Kind == domain.LedgerUpdated && e.Field == "project_id" {
			found = true
		}
	}
	if !found {
		t.Error("detach did not append a project_id ledger entry")
	}
}

// Finding: hard-deleting a document must remove its thread memberships so no
// dangling thread_item rows remain.
func TestDeleteDocumentRemovesThreadMembership(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "s", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateIssue(iss, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated}}); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	doc := domain.Document{ID: domain.NewID(), IssueID: iss.ID, Kind: domain.DocDesign, Content: "c", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateDocument(doc); err != nil {
		t.Fatalf("create document: %v", err)
	}
	th := domain.Thread{ID: domain.NewID(), Title: "t", Status: domain.ThreadActive, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateThread(th); err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if err := s.AddThreadItem(th.ID, domain.ThreadItemDocument, doc.ID); err != nil {
		t.Fatalf("attach document: %v", err)
	}

	if err := s.DeleteDocument(doc.ID); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	items, err := s.ThreadItems(th.ID)
	if err != nil {
		t.Fatalf("thread items: %v", err)
	}
	for _, it := range items {
		if it.Kind == domain.ThreadItemDocument && it.ItemID == doc.ID {
			t.Error("thread_item for deleted document was not removed")
		}
	}
}

// Finding: EncodeCWD is not injective, so two projects whose repo paths encode to
// the same folder must be rejected.
func TestCreateProjectRejectsEncodedCollision(t *testing.T) {
	s := openStore(t)
	newProject(t, s, "one", "/foo/bar-baz")

	now := time.Now().UTC()
	collide := domain.Project{
		ID: domain.NewID(), Name: "two", Slug: "two", RepoPath: "/foo-bar/baz",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateProject(collide); !errors.Is(err, domain.ErrInvalidProject) {
		t.Fatalf("colliding encoded cwd: err = %v, want ErrInvalidProject", err)
	}
}

// Finding: in a legacy database with colliding encoded paths (predating the
// collision guard), ProjectIDByEncodedCWD must return no match rather than pick
// one, regardless of insertion order.
func TestProjectIDByEncodedCWDAmbiguous(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC().Format(timeFmt)
	// Insert two colliding rows directly, bypassing CreateProject's guard.
	insert := func(id, slug, repo string) {
		if _, err := s.db.Exec(
			`INSERT INTO projects (id, name, slug, repo_path, description, created_at, updated_at)
			 VALUES (?, ?, ?, ?, '', ?, ?)`, id, slug, slug, repo, now, now); err != nil {
			t.Fatalf("raw insert %s: %v", slug, err)
		}
	}
	insert("p1", "one", "/foo/bar-baz")
	insert("p2", "two", "/foo-bar/baz") // same EncodeCWD as p1

	got, err := s.ProjectIDByEncodedCWD(domain.EncodeCWD("/foo/bar-baz"))
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if got != "" {
		t.Errorf("ambiguous encoded cwd resolved to %q, want no match", got)
	}
}

// Finding: a project with a legacy (noncanonical) slug can still update unrelated
// fields (grandfathered), but changing to another noncanonical slug is rejected.
func TestUpdateProjectGrandfathersLegacySlug(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC().Format(timeFmt)
	if _, err := s.db.Exec(
		`INSERT INTO projects (id, name, slug, repo_path, description, created_at, updated_at)
		 VALUES ('p1', 'Name', 'Legacy Slug', '/repo/x', '', ?, ?)`, now, now); err != nil {
		t.Fatalf("raw insert: %v", err)
	}

	// Unrelated update (name) must succeed despite the legacy slug.
	if _, err := s.UpdateProjectTx("p1", func(p *domain.Project) error {
		p.Name = "Renamed"
		return nil
	}); err != nil {
		t.Errorf("unrelated update blocked by legacy slug: %v", err)
	}
	// Changing to another noncanonical slug must be rejected.
	if _, err := s.UpdateProjectTx("p1", func(p *domain.Project) error {
		p.Slug = "Another Bad!"
		return nil
	}); !errors.Is(err, domain.ErrInvalidProject) {
		t.Errorf("noncanonical slug change: err = %v, want ErrInvalidProject", err)
	}
}

// Finding: creating an issue in a nonexistent project must be rejected in-tx.
func TestCreateIssueRejectsMissingProject(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	missing := "no-such-project"
	iss := domain.Issue{
		ID: domain.NewID(), Subject: "s", Body: "b",
		Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2,
		ProjectID: &missing, CreatedAt: now, UpdatedAt: now,
	}
	err := s.CreateIssue(iss, []domain.LedgerEntry{{ID: domain.NewID(), IssueID: iss.ID, At: now, Kind: domain.LedgerCreated}})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("create issue in missing project: err = %v, want ErrNotFound", err)
	}
}
