package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// contextWindow is how many messages on each side of a matching message are
// returned as adjacent context for agent synthesis.
const contextWindow = 2

// maxResults bounds a single search response (and thus the per-hit hydration
// work) so a common term over a large archive cannot produce an unbounded
// result set. A caller may request fewer via SearchQuery.Limit, never more.
const maxResults = 100

// searchIndexedKey marks in schema_meta that the current index-rebuild version
// has completed, so it is not re-scanned on every open. It is versioned: bumping
// it forces the one-time atomic rebuild to run once more on an existing DB whose
// index a prior version may have left inconsistent (v1 was an additive,
// non-repairing backfill), then never again.
const searchIndexedKey = "search_index_v2"

// migrateSearch creates the FTS5 index and the triggers that keep it in sync
// with writes to issues, documents, and messages.
//
// The index is a single ordinary FTS5 table: `kind` and `ref_id` are UNINDEXED
// columns identifying the source row, and `text` is the searchable content
// (Issue.subject+body, Document.content, Message.text). Triggers on each source
// table keep the index current without touching those tables' write paths.
//
// Soft-delete is deliberately NOT modelled in the index — it is applied at query
// time by joining hits back to the base tables (an issue's deleted_at can change
// long after its text was indexed). See Search for the exclusion rule.
func (s *Store) migrateSearch() error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(
    kind UNINDEXED,
    ref_id UNINDEXED,
    text,
    tokenize = 'porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS issues_search_ai AFTER INSERT ON issues BEGIN
    INSERT INTO search_fts(kind, ref_id, text) VALUES ('issue', new.id, new.subject || ' ' || new.body);
END;
CREATE TRIGGER IF NOT EXISTS issues_search_au AFTER UPDATE ON issues BEGIN
    DELETE FROM search_fts WHERE kind = 'issue' AND ref_id = old.id;
    INSERT INTO search_fts(kind, ref_id, text) VALUES ('issue', new.id, new.subject || ' ' || new.body);
END;
CREATE TRIGGER IF NOT EXISTS issues_search_ad AFTER DELETE ON issues BEGIN
    DELETE FROM search_fts WHERE kind = 'issue' AND ref_id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS documents_search_ai AFTER INSERT ON documents BEGIN
    INSERT INTO search_fts(kind, ref_id, text) VALUES ('document', new.id, new.content);
END;
CREATE TRIGGER IF NOT EXISTS documents_search_au AFTER UPDATE ON documents BEGIN
    DELETE FROM search_fts WHERE kind = 'document' AND ref_id = old.id;
    INSERT INTO search_fts(kind, ref_id, text) VALUES ('document', new.id, new.content);
END;
CREATE TRIGGER IF NOT EXISTS documents_search_ad AFTER DELETE ON documents BEGIN
    DELETE FROM search_fts WHERE kind = 'document' AND ref_id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS messages_search_ai AFTER INSERT ON messages BEGIN
    INSERT INTO search_fts(kind, ref_id, text) VALUES ('message', new.id, new.text);
END;
CREATE TRIGGER IF NOT EXISTS messages_search_au AFTER UPDATE ON messages BEGIN
    DELETE FROM search_fts WHERE kind = 'message' AND ref_id = old.id;
    INSERT INTO search_fts(kind, ref_id, text) VALUES ('message', new.id, new.text);
END;
CREATE TRIGGER IF NOT EXISTS messages_search_ad AFTER DELETE ON messages BEGIN
    DELETE FROM search_fts WHERE kind = 'message' AND ref_id = old.id;
END;`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate search: %w", err)
	}
	return s.reconcileSearch()
}

// reconcileSearch performs a one-time atomic rebuild of the index from the
// source tables, then records a marker so it never runs again — from then on the
// triggers keep the index current, so there is no per-open cost.
//
// It clears the index and repopulates it from issues, documents, and messages,
// so the rebuilt index exactly mirrors the source tables with no stale,
// duplicate, or orphaned rows that a prior interrupted migration (or triggers
// firing before the marker was set) might have left behind. The clear, rebuild,
// and marker all commit in one transaction, so an interrupted rebuild leaves the
// marker unset and is retried cleanly on the next open. Because it is gated by
// the marker, the full scan happens at most once.
func (s *Store) reconcileSearch() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var done int
	if err := tx.QueryRow(`SELECT count(*) FROM schema_meta WHERE key = ?`, searchIndexedKey).Scan(&done); err != nil {
		return fmt.Errorf("check search marker: %w", err)
	}
	if done > 0 {
		return nil // already rebuilt; triggers maintain the index from here
	}

	if _, err := tx.Exec(`DELETE FROM search_fts`); err != nil {
		return fmt.Errorf("clear search index: %w", err)
	}
	for _, stmt := range []string{
		`INSERT INTO search_fts(kind, ref_id, text) SELECT 'issue', id, subject || ' ' || body FROM issues`,
		`INSERT INTO search_fts(kind, ref_id, text) SELECT 'document', id, content FROM documents`,
		`INSERT INTO search_fts(kind, ref_id, text) SELECT 'message', id, text FROM messages`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("rebuild search index: %w", err)
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_meta (key, value) VALUES (?, ?)`,
		searchIndexedKey, time.Now().UTC().Format(timeFmt),
	); err != nil {
		return fmt.Errorf("set search marker: %w", err)
	}
	return tx.Commit()
}

// searchRef is a lightweight FTS match: which source row, and its relevance.
type searchRef struct {
	kind  string
	refID string
	rank  float64
}

// Search runs a full-text query and returns ranked, hydrated hits.
//
// The whole operation runs inside one deferred read transaction so matching and
// hydration observe a single consistent snapshot: a concurrent update, soft
// delete, or transcript re-ingest between the match and hydration can neither
// surface stale content nor fail hydration with ErrNotFound.
//
// Soft-delete exclusion rule (applied at match time):
//   - an issue hit is excluded if the issue is soft-deleted;
//   - a document hit is excluded if its owning issue is soft-deleted;
//   - a message hit is excluded if its transcript is linked to a soft-deleted
//     issue. A message whose transcript is unlinked is always eligible — its
//     content is not tied to any deleted issue.
//
// q.Kind (if set) restricts to one kind; q.Issue (if set) restricts to a single
// issue's content across all kinds; q.Limit bounds the result count (0 or above
// maxResults uses maxResults).
func (s *Store) Search(q domain.SearchQuery) ([]domain.SearchHit, error) {
	match := ftsQuery(q.Text)
	if match == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 || limit > maxResults {
		limit = maxResults
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	refs, err := searchRefs(tx, match, q, limit)
	if err != nil {
		return nil, err
	}
	hits := make([]domain.SearchHit, 0, len(refs))
	for _, r := range refs {
		hit, err := hydrateHit(tx, r)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return hits, nil
}

// searchRefs runs the FTS match, filters soft-deleted content and applies scope,
// and returns the ranked references. Rows are fully drained and closed before
// returning so hydration can reuse the single connection.
func searchRefs(q querier, match string, sq domain.SearchQuery, limit int) ([]searchRef, error) {
	query := `
SELECT f.kind, f.ref_id, bm25(search_fts) AS rank
FROM search_fts f
LEFT JOIN issues      i  ON f.kind = 'issue'    AND i.id  = f.ref_id
LEFT JOIN documents   d  ON f.kind = 'document' AND d.id  = f.ref_id
LEFT JOIN issues      di ON f.kind = 'document' AND di.id = d.issue_id
LEFT JOIN messages    m  ON f.kind = 'message'  AND m.id  = f.ref_id
LEFT JOIN transcripts tr ON f.kind = 'message'  AND tr.id = m.transcript_id
LEFT JOIN issues      mi ON f.kind = 'message'  AND mi.id = tr.issue_id
WHERE search_fts MATCH ?
  AND (
        (f.kind = 'issue'    AND i.deleted_at IS NULL)
     OR (f.kind = 'document' AND di.deleted_at IS NULL)
     OR (f.kind = 'message'  AND (tr.issue_id IS NULL OR mi.deleted_at IS NULL))
  )`
	args := []any{match}
	if sq.Kind != "" {
		query += " AND f.kind = ?"
		args = append(args, string(sq.Kind))
	}
	if sq.Issue != "" {
		query += ` AND (
        (f.kind = 'issue'    AND f.ref_id = ?)
     OR (f.kind = 'document' AND d.issue_id = ?)
     OR (f.kind = 'message'  AND tr.issue_id = ?)
  )`
		args = append(args, sq.Issue, sq.Issue, sq.Issue)
	}
	query += " ORDER BY rank LIMIT ?"
	args = append(args, limit)

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var refs []searchRef
	for rows.Next() {
		var r searchRef
		if err := rows.Scan(&r.kind, &r.refID, &r.rank); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

func hydrateHit(q querier, r searchRef) (domain.SearchHit, error) {
	hit := domain.SearchHit{Kind: domain.SearchKind(r.kind), Rank: r.rank}
	switch domain.SearchKind(r.kind) {
	case domain.KindIssue:
		issue, err := issueByID(q, r.refID)
		if err != nil {
			return domain.SearchHit{}, fmt.Errorf("hydrate issue hit: %w", err)
		}
		hit.Issue = &issue
	case domain.KindDocument:
		doc, err := scanDocument(q.QueryRow(`SELECT `+documentColumns+` FROM documents WHERE id = ?`, r.refID))
		if err != nil {
			return domain.SearchHit{}, fmt.Errorf("hydrate document hit: %w", err)
		}
		hit.Document = &doc
	case domain.KindMessage:
		if err := hydrateMessageHit(q, &hit, r.refID); err != nil {
			return domain.SearchHit{}, err
		}
	default:
		return domain.SearchHit{}, fmt.Errorf("unknown search kind %q", r.kind)
	}
	return hit, nil
}

func hydrateMessageHit(q querier, hit *domain.SearchHit, messageID string) error {
	msg, err := getMessage(q, messageID)
	if err != nil {
		return fmt.Errorf("hydrate message hit: %w", err)
	}
	hit.Message = &msg

	tr, err := transcriptMeta(q, msg.TranscriptID)
	if err != nil {
		return fmt.Errorf("hydrate message transcript: %w", err)
	}
	hit.Transcript = &tr

	if tr.IssueID != nil {
		linked, err := issueByID(q, *tr.IssueID)
		if err != nil {
			return fmt.Errorf("hydrate linked issue: %w", err)
		}
		hit.LinkedIssue = &linked
	}

	ctx, err := messagesAround(q, msg.TranscriptID, msg.Seq, contextWindow)
	if err != nil {
		return fmt.Errorf("hydrate message context: %w", err)
	}
	hit.Context = ctx
	return nil
}

// issueByID reads one issue against the given querier, mapping absence to
// ErrNotFound.
func issueByID(q querier, id string) (domain.Issue, error) {
	issue, err := scanIssue(q.QueryRow(`SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Issue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("get issue: %w", err)
	}
	// Hydrate labels so search issue/linked-issue hits carry the same derived
	// labels projection as GetIssue and ListIssues.
	if issue.Labels, err = labelsFor(q, id); err != nil {
		return domain.Issue{}, err
	}
	return issue, nil
}

// getMessage returns a single message by id.
func getMessage(q querier, id string) (domain.Message, error) {
	rows, err := q.Query(
		`SELECT id, transcript_id, seq, role, text, raw, at FROM messages WHERE id = ?`, id)
	if err != nil {
		return domain.Message{}, fmt.Errorf("query message: %w", err)
	}
	defer rows.Close()
	msgs, err := scanMessages(rows)
	if err != nil {
		return domain.Message{}, err
	}
	if len(msgs) == 0 {
		return domain.Message{}, domain.ErrNotFound
	}
	return msgs[0], nil
}

// transcriptMeta returns a transcript without its messages.
func transcriptMeta(q querier, id string) (domain.Transcript, error) {
	return scanTranscript(q.QueryRow(
		`SELECT id, session_id, source_path, title, issue_id, captured_at, created_at, source_mtime, project_id
		 FROM transcripts WHERE id = ?`, id))
}

// messagesAround returns the messages within +/-window seq positions of seq in
// the given transcript, excluding the message at seq itself (that message is the
// hit). Results are in seq order.
func messagesAround(q querier, transcriptID string, seq, window int) ([]domain.Message, error) {
	rows, err := q.Query(
		`SELECT id, transcript_id, seq, role, text, raw, at
		 FROM messages
		 WHERE transcript_id = ? AND seq BETWEEN ? AND ? AND seq != ?
		 ORDER BY seq`,
		transcriptID, seq-window, seq+window, seq)
	if err != nil {
		return nil, fmt.Errorf("query message context: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// scanMessages reads message rows in the standard column order.
func scanMessages(rows *sql.Rows) ([]domain.Message, error) {
	var out []domain.Message
	for rows.Next() {
		var (
			m    domain.Message
			role string
			at   sql.NullString
		)
		if err := rows.Scan(&m.ID, &m.TranscriptID, &m.Seq, &role, &m.Text, &m.Raw, &at); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.Role = domain.Role(role)
		if at.Valid {
			ts, err := time.Parse(timeFmt, at.String)
			if err != nil {
				return nil, fmt.Errorf("parse message at: %w", err)
			}
			m.At = &ts
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ftsQuery turns free-text into a safe FTS5 MATCH expression: each whitespace-
// separated token is double-quoted (with internal quotes doubled) so arbitrary
// user input can never be an FTS syntax error, and multiple tokens combine with
// implicit AND. Returns "" when the input has no tokens.
func ftsQuery(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " ")
}
