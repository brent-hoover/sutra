package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

func (s *Store) migrateTranscripts() error {
	const schema = `
CREATE TABLE IF NOT EXISTS transcripts (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL UNIQUE,
    source_path TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    issue_id    TEXT,
    captured_at TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
    id            TEXT PRIMARY KEY,
    transcript_id TEXT NOT NULL,
    seq           INTEGER NOT NULL,
    role          TEXT NOT NULL,
    text          TEXT NOT NULL DEFAULT '',
    raw           TEXT NOT NULL,
    at            TEXT,
    UNIQUE(transcript_id, seq)
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate transcripts: %w", err)
	}
	return nil
}

// UpsertTranscript stores a transcript and its messages, keyed by session_id.
// Re-ingesting the same session_id updates the existing Transcript in place and
// replaces its Message rows (idempotent — never duplicated). The stored
// transcript, including its persisted id and messages, is returned.
func (s *Store) UpsertTranscript(t domain.Transcript) (domain.Transcript, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Transcript{}, err
	}
	defer tx.Rollback()

	// Reuse the existing row (id + created_at) when the session already exists.
	// issue_id is deliberately left untouched by the UPDATE below so that
	// re-ingesting a linked transcript preserves its issue link.
	var (
		existingID, existingCreated string
		existingIssueID             sql.NullString
	)
	err = tx.QueryRow(`SELECT id, created_at, issue_id FROM transcripts WHERE session_id = ?`, t.SessionID).
		Scan(&existingID, &existingCreated, &existingIssueID)
	switch {
	case err == nil:
		t.ID = existingID
		if t.CreatedAt, err = time.Parse(timeFmt, existingCreated); err != nil {
			return domain.Transcript{}, fmt.Errorf("parse created_at: %w", err)
		}
		if _, err := tx.Exec(
			`UPDATE transcripts SET source_path = ?, title = ?, captured_at = ? WHERE id = ?`,
			t.SourcePath, t.Title, t.CapturedAt.Format(timeFmt), t.ID,
		); err != nil {
			return domain.Transcript{}, fmt.Errorf("update transcript: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(
			`INSERT INTO transcripts (id, session_id, source_path, title, issue_id, captured_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.SessionID, t.SourcePath, t.Title, nullString(t.IssueID),
			t.CapturedAt.Format(timeFmt), t.CreatedAt.Format(timeFmt),
		); err != nil {
			return domain.Transcript{}, fmt.Errorf("insert transcript: %w", err)
		}
	default:
		return domain.Transcript{}, fmt.Errorf("lookup transcript: %w", err)
	}

	// Upsert messages by (transcript_id, seq) so a re-ingest updates rows in
	// place, preserving each existing message's id rather than recreating it.
	for i := range t.Messages {
		m := &t.Messages[i]
		m.TranscriptID = t.ID
		if m.ID == "" {
			m.ID = domain.NewID()
		}
		if _, err := tx.Exec(
			`INSERT INTO messages (id, transcript_id, seq, role, text, raw, at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(transcript_id, seq) DO UPDATE SET
			     role = excluded.role, text = excluded.text, raw = excluded.raw, at = excluded.at`,
			m.ID, m.TranscriptID, m.Seq, string(m.Role), m.Text, m.Raw, nullTime(m.At),
		); err != nil {
			return domain.Transcript{}, fmt.Errorf("upsert message: %w", err)
		}
	}
	// Drop any trailing rows left over from a previous, longer ingest.
	if _, err := tx.Exec(
		`DELETE FROM messages WHERE transcript_id = ? AND seq >= ?`, t.ID, len(t.Messages),
	); err != nil {
		return domain.Transcript{}, fmt.Errorf("prune messages: %w", err)
	}

	// Re-ingesting a linked transcript changes content tied to its issue, so
	// advance that issue's updated_at and record it in the ledger, in the same tx.
	if existingIssueID.Valid {
		now := time.Now().UTC()
		if _, err := tx.Exec(
			`UPDATE issues SET updated_at = ? WHERE id = ?`,
			now.Format(timeFmt), existingIssueID.String,
		); err != nil {
			return domain.Transcript{}, fmt.Errorf("bump issue updated_at: %w", err)
		}
		if err := insertLedger(tx, []domain.LedgerEntry{{
			ID: domain.NewID(), IssueID: existingIssueID.String, At: now,
			Kind: domain.LedgerUpdated, Field: "transcript", NewValue: "re-ingested " + t.SessionID,
		}}); err != nil {
			return domain.Transcript{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.Transcript{}, err
	}
	// Return the stored state so the caller sees the persisted issue_id link and
	// the preserved message ids, not the freshly-generated in-memory values.
	return s.GetTranscript(t.ID)
}

// GetTranscript returns the transcript with the given id and its messages in
// seq order, or ErrNotFound.
func (s *Store) GetTranscript(id string) (domain.Transcript, error) {
	t, err := scanTranscript(s.db.QueryRow(
		`SELECT id, session_id, source_path, title, issue_id, captured_at, created_at
		 FROM transcripts WHERE id = ?`, id))
	if err != nil {
		return domain.Transcript{}, err
	}
	if t.Messages, err = s.messagesFor(t.ID); err != nil {
		return domain.Transcript{}, err
	}
	return t, nil
}

// LinkTranscript sets a transcript's issue_id and appends the ledger entry to
// the issue, in one transaction. Both the transcript and the issue must exist.
func (s *Store) LinkTranscript(transcriptID, issueID string, entry domain.LedgerEntry) (domain.Transcript, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Transcript{}, err
	}
	defer tx.Rollback()

	// The transcript must exist; reject relinking it to a different issue (that
	// would silently drop it from the prior issue with no audit trail there).
	var current sql.NullString
	if err := tx.QueryRow(`SELECT issue_id FROM transcripts WHERE id = ?`, transcriptID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return domain.Transcript{}, domain.ErrNotFound
	} else if err != nil {
		return domain.Transcript{}, fmt.Errorf("lookup transcript: %w", err)
	}
	if current.Valid {
		if current.String == issueID {
			// Already linked to this issue: idempotent — no new ledger entry and
			// no updated_at bump. Release the connection before reading back
			// (the single-connection pool is held by this tx).
			_ = tx.Rollback()
			return s.GetTranscript(transcriptID)
		}
		return domain.Transcript{}, errors.Join(domain.ErrInvalidTranscript,
			fmt.Errorf("transcript already linked to issue %s", current.String))
	}

	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM issues WHERE id = ?`, issueID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return domain.Transcript{}, domain.ErrNotFound
	} else if err != nil {
		return domain.Transcript{}, fmt.Errorf("lookup issue: %w", err)
	}

	if _, err := tx.Exec(`UPDATE transcripts SET issue_id = ? WHERE id = ?`, issueID, transcriptID); err != nil {
		return domain.Transcript{}, fmt.Errorf("link transcript: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO ledger (id, issue_id, at, actor, kind, field, old_value, new_value)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.IssueID, entry.At.Format(timeFmt), entry.Actor, string(entry.Kind),
		entry.Field, entry.OldValue, entry.NewValue,
	); err != nil {
		return domain.Transcript{}, fmt.Errorf("insert ledger: %w", err)
	}

	// Linking is a change to the issue: advance its updated_at in the same tx,
	// with a timestamp generated here so it cannot regress.
	if _, err := tx.Exec(
		`UPDATE issues SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(timeFmt), issueID,
	); err != nil {
		return domain.Transcript{}, fmt.Errorf("bump issue updated_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.Transcript{}, err
	}
	return s.GetTranscript(transcriptID)
}

// TranscriptsForIssue returns the transcripts linked to an issue (without
// messages), ordered by capture time.
func (s *Store) TranscriptsForIssue(issueID string) ([]domain.Transcript, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, source_path, title, issue_id, captured_at, created_at
		 FROM transcripts WHERE issue_id = ? ORDER BY captured_at`, issueID)
	if err != nil {
		return nil, fmt.Errorf("query transcripts: %w", err)
	}
	defer rows.Close()

	var out []domain.Transcript
	for rows.Next() {
		t, err := scanTranscript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// IngestedSessionIDs returns the set of session_ids already stored.
func (s *Store) IngestedSessionIDs() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT session_id FROM transcripts`)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	set := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		set[id] = true
	}
	return set, rows.Err()
}

func scanTranscript(row rowScanner) (domain.Transcript, error) {
	var (
		t                     domain.Transcript
		issueID               sql.NullString
		capturedAt, createdAt string
	)
	err := row.Scan(&t.ID, &t.SessionID, &t.SourcePath, &t.Title, &issueID, &capturedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Transcript{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Transcript{}, fmt.Errorf("scan transcript: %w", err)
	}
	if issueID.Valid {
		t.IssueID = &issueID.String
	}
	if t.CapturedAt, err = time.Parse(timeFmt, capturedAt); err != nil {
		return domain.Transcript{}, fmt.Errorf("parse captured_at: %w", err)
	}
	if t.CreatedAt, err = time.Parse(timeFmt, createdAt); err != nil {
		return domain.Transcript{}, fmt.Errorf("parse created_at: %w", err)
	}
	return t, nil
}

func (s *Store) messagesFor(transcriptID string) ([]domain.Message, error) {
	rows, err := s.db.Query(
		`SELECT id, transcript_id, seq, role, text, raw, at
		 FROM messages WHERE transcript_id = ? ORDER BY seq`, transcriptID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}
