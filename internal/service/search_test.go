package service_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/brent-hoover/sutra/internal/service"
)

// messageLines returns a transcript whose seq==2 message contains term.
func messageLines(term string) []string {
	return []string{
		`{"type":"user","message":{"role":"user","content":"first message"},"timestamp":"2026-07-16T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"second message"}]},"timestamp":"2026-07-16T10:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":"the ` + term + ` middle message"},"timestamp":"2026-07-16T10:00:02Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"fourth message"}]},"timestamp":"2026-07-16T10:00:03Z"}`,
		`{"type":"user","message":{"role":"user","content":"fifth message"},"timestamp":"2026-07-16T10:00:04Z"}`,
	}
}

func hitKinds(res domain.SearchResults) map[domain.SearchKind]int {
	counts := map[domain.SearchKind]int{}
	for _, h := range res.Hits {
		counts[h.Kind]++
	}
	return counts
}

// TestSearchAcrossEntities: one query returns hits from an issue, a document,
// and a transcript message.
func TestSearchAcrossEntities(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	const term = "flibbertigibbet"

	iss, err := svc.CreateIssue("cross entity", "body mentions "+term)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	host, err := svc.CreateIssue("doc host", "no term here")
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	if _, err := svc.AttachDocument(host.ID, domain.DocDesign, "Design", "content with "+term); err != nil {
		t.Fatalf("attach doc: %v", err)
	}
	path := writeSession(t, filepath.Join(projectsDir, "-Users-me-x"), "cross-session", messageLines(term))
	if _, err := svc.IngestTranscript(path); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	res, err := svc.Search(domain.SearchQuery{Text: term})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	counts := hitKinds(res)
	if counts[domain.KindIssue] < 1 || counts[domain.KindDocument] < 1 || counts[domain.KindMessage] < 1 {
		t.Fatalf("expected a hit of each kind, got %v", counts)
	}
	// The issue hit must be the one whose body carried the term.
	var found bool
	for _, h := range res.Hits {
		if h.Kind == domain.KindIssue && h.Issue != nil && h.Issue.ID == iss.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("issue hit %s not found", iss.ID)
	}
}

// TestSearchExcludesSoftDeleted verifies the full soft-delete exclusion rule:
// a soft-deleted issue's own row, its documents, and messages of transcripts
// linked to it are all excluded; an unlinked transcript's message survives.
func TestSearchExcludesSoftDeleted(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	const term = "vorpalsword"

	// Live issue with the term + a document.
	live, err := svc.CreateIssue("live", "body with "+term)
	if err != nil {
		t.Fatalf("create live: %v", err)
	}
	if _, err := svc.AttachDocument(live.ID, domain.DocPlan, "live doc", "doc with "+term); err != nil {
		t.Fatalf("attach live doc: %v", err)
	}

	// Doomed issue with the term + a document + a linked transcript.
	doomed, err := svc.CreateIssue("doomed", "body with "+term)
	if err != nil {
		t.Fatalf("create doomed: %v", err)
	}
	if _, err := svc.AttachDocument(doomed.ID, domain.DocPlan, "doomed doc", "doc with "+term); err != nil {
		t.Fatalf("attach doomed doc: %v", err)
	}
	doomedPath := writeSession(t, filepath.Join(projectsDir, "-Users-me-doomed"), "doomed-session", messageLines(term))
	doomedTr, err := svc.IngestTranscript(doomedPath)
	if err != nil {
		t.Fatalf("ingest doomed: %v", err)
	}
	if _, err := svc.LinkTranscript(doomedTr.ID, doomed.ID); err != nil {
		t.Fatalf("link doomed transcript: %v", err)
	}

	// Unlinked transcript with the term — must survive the delete.
	freePath := writeSession(t, filepath.Join(projectsDir, "-Users-me-free"), "free-session", messageLines(term))
	if _, err := svc.IngestTranscript(freePath); err != nil {
		t.Fatalf("ingest free: %v", err)
	}

	if _, err := svc.SoftDeleteIssue(doomed.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	res, err := svc.Search(domain.SearchQuery{Text: term})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, h := range res.Hits {
		switch h.Kind {
		case domain.KindIssue:
			if h.Issue.ID == doomed.ID {
				t.Errorf("soft-deleted issue %s appeared in results", doomed.ID)
			}
		case domain.KindDocument:
			if h.Document.IssueID == doomed.ID {
				t.Errorf("document of soft-deleted issue appeared in results")
			}
		case domain.KindMessage:
			if h.LinkedIssue != nil && h.LinkedIssue.ID == doomed.ID {
				t.Errorf("message of transcript linked to soft-deleted issue appeared")
			}
		}
	}
	// Positive controls: the live issue, its doc, and the unlinked message survive.
	counts := hitKinds(res)
	if counts[domain.KindIssue] != 1 {
		t.Errorf("expected exactly the live issue, got %d issue hits", counts[domain.KindIssue])
	}
	if counts[domain.KindDocument] != 1 {
		t.Errorf("expected exactly the live document, got %d document hits", counts[domain.KindDocument])
	}
	if counts[domain.KindMessage] != 1 {
		t.Errorf("expected exactly the unlinked message, got %d message hits", counts[domain.KindMessage])
	}
}

// TestSearchRanked: more-relevant content ranks first, and ranks are ascending.
func TestSearchRanked(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	const term = "brillig"

	host, err := svc.CreateIssue("ranking host", "no term")
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	if _, err := svc.AttachDocument(host.ID, domain.DocProblem, "sparse", "mentions "+term+" once"); err != nil {
		t.Fatalf("attach sparse: %v", err)
	}
	dense, err := svc.AttachDocument(host.ID, domain.DocDesign, "dense", strings.Repeat(term+" ", 8)+"tail")
	if err != nil {
		t.Fatalf("attach dense: %v", err)
	}

	res, err := svc.Search(domain.SearchQuery{Text: term})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) < 2 {
		t.Fatalf("expected >=2 hits, got %d", len(res.Hits))
	}
	for i := 1; i < len(res.Hits); i++ {
		if res.Hits[i].Rank < res.Hits[i-1].Rank {
			t.Errorf("hits not ordered by rank at %d", i)
		}
	}
	if res.Hits[0].Kind != domain.KindDocument || res.Hits[0].Document.ID != dense.ID {
		t.Errorf("dense document should rank first, got %+v", res.Hits[0])
	}
}

// TestSearchScoped: kind and issue scopes narrow results.
func TestSearchScoped(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	const term = "slithytove"

	a, err := svc.CreateIssue("issue a", "body with "+term)
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := svc.AttachDocument(a.ID, domain.DocPlan, "doc a", "content with "+term); err != nil {
		t.Fatalf("attach a: %v", err)
	}
	b, err := svc.CreateIssue("issue b", "body with "+term)
	if err != nil {
		t.Fatalf("create b: %v", err)
	}

	// Kind scope: only documents.
	byKind, err := svc.Search(domain.SearchQuery{Text: term, Kind: domain.KindDocument})
	if err != nil {
		t.Fatalf("kind search: %v", err)
	}
	if len(byKind.Hits) == 0 {
		t.Fatal("kind-scoped search returned nothing")
	}
	for _, h := range byKind.Hits {
		if h.Kind != domain.KindDocument {
			t.Errorf("kind=document returned a %s hit", h.Kind)
		}
	}

	// Issue scope: only issue b's content (its issue row; it has no docs).
	byIssue, err := svc.Search(domain.SearchQuery{Text: term, Issue: b.ID})
	if err != nil {
		t.Fatalf("issue search: %v", err)
	}
	if len(byIssue.Hits) != 1 || byIssue.Hits[0].Kind != domain.KindIssue || byIssue.Hits[0].Issue.ID != b.ID {
		t.Errorf("issue-scoped search should return only issue %s, got %+v", b.ID, byIssue.Hits)
	}
}

// TestSearchMessageContext: a message hit carries role/text/seq, its owning
// transcript, the linked issue, and a window of adjacent messages (excluding
// the hit itself).
func TestSearchMessageContext(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	const term = "jabberwocky"

	path := writeSession(t, filepath.Join(projectsDir, "-Users-me-ctx"), "ctx-session", messageLines(term))
	tr, err := svc.IngestTranscript(path)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	iss, err := svc.CreateIssue("owner", "issue that owns the transcript")
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if _, err := svc.LinkTranscript(tr.ID, iss.ID); err != nil {
		t.Fatalf("link: %v", err)
	}

	res, err := svc.Search(domain.SearchQuery{Text: term})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var hit *domain.SearchHit
	for i := range res.Hits {
		if res.Hits[i].Kind == domain.KindMessage {
			hit = &res.Hits[i]
			break
		}
	}
	if hit == nil {
		t.Fatal("no message hit")
	}
	if hit.Message == nil || hit.Message.Seq != 2 || hit.Message.Role == "" {
		t.Fatalf("message hit missing role/seq: %+v", hit.Message)
	}
	if !strings.Contains(hit.Message.Text, term) {
		t.Errorf("message text does not contain term: %q", hit.Message.Text)
	}
	if hit.Transcript == nil || hit.Transcript.ID != tr.ID {
		t.Errorf("message hit missing owning transcript")
	}
	if hit.LinkedIssue == nil || hit.LinkedIssue.ID != iss.ID {
		t.Errorf("message hit missing linked issue")
	}
	seqs := map[int]bool{}
	for _, c := range hit.Context {
		seqs[c.Seq] = true
	}
	if !seqs[1] || !seqs[3] {
		t.Errorf("context missing adjacent seqs 1 and 3, got %v", seqs)
	}
	if seqs[2] {
		t.Errorf("context must exclude the matching message (seq 2)")
	}
}

// TestSearchValidation: empty text and an unknown kind are rejected.
func TestSearchValidation(t *testing.T) {
	svc := newService(t, t.TempDir())

	if _, err := svc.Search(domain.SearchQuery{Text: "   "}); !errors.Is(err, domain.ErrInvalidSearch) {
		t.Errorf("empty text: err = %v, want ErrInvalidSearch", err)
	}
	if _, err := svc.Search(domain.SearchQuery{Text: "x", Kind: domain.SearchKind("bogus")}); !errors.Is(err, domain.ErrInvalidSearch) {
		t.Errorf("bad kind: err = %v, want ErrInvalidSearch", err)
	}
}

// TestSearchIndexSyncOnDocumentWrites: the FTS index tracks document content
// through update and removal (exercising the update/delete triggers).
func TestSearchIndexSyncOnDocumentWrites(t *testing.T) {
	svc := newService(t, t.TempDir())
	const term = "mimsyborogove"

	iss, err := svc.CreateIssue("host", "no term")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	doc, err := svc.AttachDocument(iss.ID, domain.DocPlan, "doc", "content without the term")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if hits := searchDocs(t, svc, term); len(hits) != 0 {
		t.Fatalf("term found before it was added, got %d hits", len(hits))
	}

	// Update to add the term — the AU trigger must re-index.
	if _, err := svc.UpdateDocument(doc.ID, "now the "+term+" is present"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if hits := searchDocs(t, svc, term); len(hits) != 1 {
		t.Fatalf("after update: got %d document hits, want 1", len(hits))
	}

	// Update to drop the term — the AU trigger must remove the stale entry.
	if _, err := svc.UpdateDocument(doc.ID, "the term is gone again"); err != nil {
		t.Fatalf("update 2: %v", err)
	}
	if hits := searchDocs(t, svc, term); len(hits) != 0 {
		t.Fatalf("after removing term via update: got %d hits, want 0", len(hits))
	}

	// Re-add then remove the document — the AD trigger must drop the entry.
	if _, err := svc.UpdateDocument(doc.ID, "the "+term+" returns"); err != nil {
		t.Fatalf("update 3: %v", err)
	}
	if err := svc.RemoveDocument(doc.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if hits := searchDocs(t, svc, term); len(hits) != 0 {
		t.Fatalf("after remove: got %d hits, want 0", len(hits))
	}
}

// TestSearchIndexSyncOnReingestPrune: re-ingesting a shorter transcript prunes
// the dropped message from the index (exercising the message delete trigger via
// the prune path).
func TestSearchIndexSyncOnReingestPrune(t *testing.T) {
	projectsDir := t.TempDir()
	svc := newService(t, projectsDir)
	const term = "outgrabe"

	dir := filepath.Join(projectsDir, "-Users-me-prune")
	path := writeSession(t, dir, "prune-session", messageLines(term))
	if _, err := svc.IngestTranscript(path); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if hits := searchKind(t, svc, term, domain.KindMessage); len(hits) != 1 {
		t.Fatalf("before re-ingest: got %d message hits, want 1", len(hits))
	}

	// Re-ingest a shorter transcript that drops the message carrying the term.
	shorter := []string{
		`{"type":"user","message":{"role":"user","content":"only message"},"timestamp":"2026-07-16T10:00:00Z"}`,
	}
	writeSession(t, dir, "prune-session", shorter)
	if _, err := svc.IngestTranscript(path); err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if hits := searchKind(t, svc, term, domain.KindMessage); len(hits) != 0 {
		t.Fatalf("after prune: got %d message hits, want 0", len(hits))
	}
}

func searchDocs(t *testing.T, svc *service.Service, term string) []domain.SearchHit {
	t.Helper()
	return searchKind(t, svc, term, domain.KindDocument)
}

func searchKind(t *testing.T, svc *service.Service, term string, kind domain.SearchKind) []domain.SearchHit {
	t.Helper()
	res, err := svc.Search(domain.SearchQuery{Text: term, Kind: kind})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	return res.Hits
}

// TestSearchNoResults returns an empty (non-nil) hit list.
func TestSearchNoResults(t *testing.T) {
	svc := newService(t, t.TempDir())
	res, err := svc.Search(domain.SearchQuery{Text: "nothingmatchesthisxyz"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Hits == nil {
		t.Error("expected non-nil empty hits slice")
	}
	if len(res.Hits) != 0 {
		t.Errorf("expected no hits, got %d", len(res.Hits))
	}
}
