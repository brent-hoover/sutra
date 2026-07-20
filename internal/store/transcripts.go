package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

func (s *Store) migrateTranscripts() error {
	const schema = `
CREATE TABLE IF NOT EXISTS transcripts (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL UNIQUE,
    source_path  TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    issue_id     TEXT,
    captured_at  TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    source_mtime TEXT NOT NULL DEFAULT ''
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
	// Additive migration for databases created before source_mtime existed.
	if err := s.addColumnIfMissing("transcripts", "source_mtime", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate transcripts source_mtime: %w", err)
	}
	return nil
}

// addColumnIfMissing adds a column to a table when it is not already present,
// so additive schema changes are idempotent across existing databases.
func (s *Store) addColumnIfMissing(table, column, decl string) error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Close()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
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
		existingMtime               string
		existingIssueID             sql.NullString
	)
	// logReingest records whether this upsert is a genuine content change of a
	// previously-recorded, issue-linked session, and thus warrants a ledger
	// entry + issue bump below. It stays false for first ingests and for a
	// first-time source_mtime backfill of a pre-source_mtime (migrated) row.
	logReingest := false
	err = tx.QueryRow(`SELECT id, created_at, issue_id, source_mtime FROM transcripts WHERE session_id = ?`, t.SessionID).
		Scan(&existingID, &existingCreated, &existingIssueID, &existingMtime)
	switch {
	case err == nil:
		_ = existingMtime // mtime is a feed hint, not the change-detection authority
		t.ID = existingID
		if t.CreatedAt, err = time.Parse(timeFmt, existingCreated); err != nil {
			return domain.Transcript{}, fmt.Errorf("parse created_at: %w", err)
		}
		// Content-authoritative change detection (inside the tx): compare the
		// incoming messages against the stored rows. File mtime is not trusted as
		// proof of equality — a touched-but-unchanged file is a no-op, and a
		// changed file is detected even if its mtime was preserved. Being in the
		// tx makes this safe against concurrent activity requests.
		prevSig, err := storedContentSignature(tx, existingID)
		if err != nil {
			return domain.Transcript{}, err
		}
		if prevSig == contentSignature(t.Messages) {
			_ = tx.Rollback()
			return s.GetTranscript(t.ID)
		}
		if _, err := tx.Exec(
			`UPDATE transcripts SET source_path = ?, title = ?, captured_at = ?, source_mtime = ? WHERE id = ?`,
			t.SourcePath, t.Title, t.CapturedAt.Format(timeFmt), mtimeStr(t.SourceMtime), t.ID,
		); err != nil {
			return domain.Transcript{}, fmt.Errorf("update transcript: %w", err)
		}
		// The content genuinely changed; if the session is linked, that changes
		// data tied to its issue, so record it.
		logReingest = existingIssueID.Valid
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(
			`INSERT INTO transcripts (id, session_id, source_path, title, issue_id, captured_at, created_at, source_mtime)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.SessionID, t.SourcePath, t.Title, nullString(t.IssueID),
			t.CapturedAt.Format(timeFmt), t.CreatedAt.Format(timeFmt), mtimeStr(t.SourceMtime),
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

	// Re-ingesting a linked transcript with genuinely new content changes data
	// tied to its issue, so advance that issue's updated_at and record it in the
	// ledger, in the same tx. Unchanged re-ingests and backfills do not reach here.
	if logReingest {
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
		`SELECT id, session_id, source_path, title, issue_id, captured_at, created_at, source_mtime
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
		`SELECT id, session_id, source_path, title, issue_id, captured_at, created_at, source_mtime
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
		sourceMtime           string
	)
	err := row.Scan(&t.ID, &t.SessionID, &t.SourcePath, &t.Title, &issueID, &capturedAt, &createdAt, &sourceMtime)
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
	if t.SourceMtime, err = parseMtime(sourceMtime); err != nil {
		return domain.Transcript{}, fmt.Errorf("parse source_mtime: %w", err)
	}
	return t, nil
}

// contentSignature is a stable fingerprint of a transcript's messages (seq +
// raw line). Two ingests produce the same signature iff their content matches,
// so re-ingest change detection does not have to trust file mtimes.
func contentSignature(msgs []domain.Message) string {
	h := sha256.New()
	for _, m := range msgs {
		fmt.Fprintf(h, "%d\n%s\n", m.Seq, m.Raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// storedContentSignature computes contentSignature over the messages already
// persisted for a transcript, read through the given transaction.
func storedContentSignature(tx *sql.Tx, transcriptID string) (string, error) {
	rows, err := tx.Query(`SELECT seq, raw FROM messages WHERE transcript_id = ? ORDER BY seq`, transcriptID)
	if err != nil {
		return "", fmt.Errorf("read stored messages: %w", err)
	}
	defer rows.Close()
	var msgs []domain.Message
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.Seq, &m.Raw); err != nil {
			return "", fmt.Errorf("scan stored message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return contentSignature(msgs), nil
}

// mtimeStr formats a source mtime for storage; a zero time (unknown) is stored
// as the empty string.
func mtimeStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeFmt)
}

// parseMtime is the inverse of mtimeStr: an empty string is the zero time.
func parseMtime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(timeFmt, s)
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
