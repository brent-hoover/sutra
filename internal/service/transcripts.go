package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/brent-hoover/sutra/internal/domain"
)

// maxTitleLen bounds the derived title so a long first message doesn't blow up
// list rendering.
const maxTitleLen = 120

// IngestTranscript parses a Claude .jsonl session file and stores it as a
// Transcript with one Message per line. It is idempotent on session_id (see
// store.UpsertTranscript). The stored transcript, with its messages, is
// returned.
func (s *Service) IngestTranscript(path string) (domain.Transcript, error) {
	// Confine ingest to a .jsonl file within the pinned projects directory. The
	// projectsRoot handle (opened once at construction) makes this TOCTOU-safe:
	// neither the projects dir nor any symlink component can be swapped to
	// escape it between check and open.
	rel, err := s.relWithinProjects(path)
	if err != nil {
		return domain.Transcript{}, err
	}
	if !strings.HasSuffix(rel, ".jsonl") {
		return domain.Transcript{}, errors.Join(domain.ErrInvalidTranscript,
			fmt.Errorf("path %q is not a .jsonl file", path))
	}

	f, err := s.projectsRoot.Open(rel)
	if err != nil {
		return domain.Transcript{}, errors.Join(domain.ErrInvalidTranscript,
			fmt.Errorf("open transcript %q: %w", path, err))
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return domain.Transcript{}, fmt.Errorf("stat transcript: %w", err)
	}
	if !info.Mode().IsRegular() {
		return domain.Transcript{}, errors.Join(domain.ErrInvalidTranscript,
			fmt.Errorf("path %q is not a regular file", path))
	}

	now := time.Now().UTC()
	t := domain.Transcript{
		ID:          domain.NewID(),
		SessionID:   sessionIDFromPath(rel),
		SourcePath:  filepath.Join(s.projectsDir, rel),
		CreatedAt:   now,
		SourceMtime: info.ModTime().UTC(),
	}

	// Read line-by-line with a bufio.Reader (no per-line size cap, unlike
	// bufio.Scanner) so an arbitrarily large JSONL line is still captured.
	reader := bufio.NewReader(f)
	seq := 0
	for {
		raw, readErr := reader.ReadString('\n')
		if len(raw) > 0 {
			// Ingestion is lossless: every line becomes exactly one Message and
			// its Raw is the line content (delimiter stripped, blanks preserved).
			line := strings.TrimSuffix(strings.TrimSuffix(raw, "\n"), "\r")
			msg, at := parseLine(line, seq)
			t.Messages = append(t.Messages, msg)
			// captured_at and title come from the first event that supplies them.
			if t.CapturedAt.IsZero() && at != nil {
				t.CapturedAt = *at
			}
			if t.Title == "" && msg.Role == domain.RoleUser {
				t.Title = deriveTitle(msg.Text)
			}
			seq++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return domain.Transcript{}, fmt.Errorf("read transcript: %w", readErr)
		}
	}

	// Fall back to file mtime when no event carried a timestamp.
	if t.CapturedAt.IsZero() {
		t.CapturedAt = info.ModTime().UTC()
	}

	if err := t.Validate(); err != nil {
		return domain.Transcript{}, err
	}
	return s.store.UpsertTranscript(t)
}

// GetTranscript returns a transcript and its messages in seq order.
func (s *Service) GetTranscript(id string) (domain.Transcript, error) {
	return s.store.GetTranscript(id)
}

// LinkTranscript links a transcript to an issue and appends a "linked" ledger
// entry to that issue.
func (s *Service) LinkTranscript(transcriptID, issueID string) (domain.Transcript, error) {
	entry := domain.LedgerEntry{
		ID:       domain.NewID(),
		IssueID:  issueID,
		At:       time.Now().UTC(),
		Kind:     domain.LedgerLinked,
		Field:    "transcript_id",
		NewValue: transcriptID,
	}
	return s.store.LinkTranscript(transcriptID, issueID, entry)
}

// TranscriptsForIssue returns the transcripts linked to an issue. It returns
// ErrNotFound if the issue does not exist (so callers can 404 rather than
// return an empty list for a bogus id).
func (s *Service) TranscriptsForIssue(issueID string) ([]domain.Transcript, error) {
	if _, err := s.store.GetIssue(issueID); err != nil {
		return nil, err
	}
	return s.store.TranscriptsForIssue(issueID)
}

// DiscoverTranscripts lists Claude session files. With dir empty it scans
// ~/.claude/projects; otherwise it scans the given project dir. Each file's
// session_id, path, and ingested state is returned.
func (s *Service) DiscoverTranscripts(dir string) ([]domain.DiscoveredTranscript, error) {
	// Walk the pinned projects root (TOCTOU-safe). An empty dir scans the whole
	// projects dir; a caller-supplied dir must live within it.
	sub := "."
	if dir != "" {
		rel, err := s.relWithinProjects(dir)
		if err != nil {
			return nil, err
		}
		sub = filepath.ToSlash(rel)
	}
	ingested, err := s.store.IngestedSessionIDs()
	if err != nil {
		return nil, err
	}

	var out []domain.DiscoveredTranscript
	walkErr := fs.WalkDir(s.projectsRoot.FS(), sub, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // a missing subdir is simply "nothing to discover"
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		sid := sessionIDFromPath(p)
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat transcript %s: %w", p, err)
		}
		out = append(out, domain.DiscoveredTranscript{
			SessionID: sid,
			Path:      filepath.Join(s.projectsDir, filepath.FromSlash(p)),
			Ingested:  ingested[sid],
			ModTime:   info.ModTime().UTC(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("discover transcripts: %w", walkErr)
	}
	return out, nil
}

// sessionIDFromPath returns the JSONL filename without its extension — the
// Claude session UUID.
func sessionIDFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

// relWithinProjects returns path expressed relative to the projects directory,
// wrapping ErrInvalidTranscript if it escapes that directory. It is a textual
// containment check; the pinned projectsRoot enforces symlink safety at open.
func (s *Service) relWithinProjects(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Join(domain.ErrInvalidTranscript, err)
	}
	rel, err := filepath.Rel(s.projectsDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.Join(domain.ErrInvalidTranscript,
			fmt.Errorf("path %q is outside the projects directory", path))
	}
	return rel, nil
}

func deriveTitle(text string) string {
	title := strings.TrimSpace(text)
	if i := strings.IndexByte(title, '\n'); i >= 0 {
		title = title[:i]
	}
	// Truncate on a rune boundary so a multibyte character is never split.
	if utf8.RuneCountInString(title) > maxTitleLen {
		title = strings.TrimSpace(string([]rune(title)[:maxTitleLen]))
	}
	return title
}

// rawEvent is the subset of a Claude JSONL line we interpret. Everything else
// is preserved losslessly in Message.Raw.
type rawEvent struct {
	Type      string      `json:"type"`
	Role      string      `json:"role"`
	Timestamp string      `json:"timestamp"`
	Message   *rawMessage `json:"message"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// parseLine extracts role, text, and timestamp from one JSONL line. The raw
// line is always preserved. A line that isn't valid JSON is still captured (as
// a system message) so ingestion never drops data.
func parseLine(line string, seq int) (domain.Message, *time.Time) {
	msg := domain.Message{
		ID:   domain.NewID(),
		Seq:  seq,
		Raw:  line,
		Role: domain.RoleSystem,
	}
	var ev rawEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return msg, nil
	}

	rawRole := ev.Role
	var content json.RawMessage
	if ev.Message != nil {
		if ev.Message.Role != "" {
			rawRole = ev.Message.Role
		}
		content = ev.Message.Content
	}
	if rawRole == "" {
		rawRole = ev.Type
	}

	text, hasText, hasToolUse, hasToolResult := extractContent(content)
	msg.Text = text
	msg.Role = classifyRole(rawRole, hasText, hasToolUse, hasToolResult)

	if ev.Timestamp != "" {
		if at := parseTimestamp(ev.Timestamp); at != nil {
			msg.At = at
			return msg, at
		}
	}
	return msg, nil
}

func classifyRole(rawRole string, hasText, hasToolUse, hasToolResult bool) domain.Role {
	switch rawRole {
	case "user":
		// A tool_result is a tool turn even when it carries output text.
		if hasToolResult {
			return domain.RoleTool
		}
		return domain.RoleUser
	case "assistant":
		if hasToolUse && !hasText {
			return domain.RoleTool
		}
		return domain.RoleAssistant
	case "system":
		return domain.RoleSystem
	default:
		return domain.RoleSystem
	}
}

// extractContent pulls plain text out of a Claude content value, which is
// either a JSON string or an array of typed blocks.
func extractContent(content json.RawMessage) (text string, hasText, hasToolUse, hasToolResult bool) {
	if len(content) == 0 {
		return "", false, false, false
	}
	// Content as a bare string.
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		return str, str != "", false, false
	}
	// Content as an array of blocks.
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return "", false, false, false
	}
	var parts []string
	for _, b := range blocks {
		var typ string
		_ = json.Unmarshal(b["type"], &typ)
		switch typ {
		case "text":
			var s string
			if json.Unmarshal(b["text"], &s) == nil && s != "" {
				parts = append(parts, s)
			}
		case "tool_use":
			hasToolUse = true
		case "tool_result":
			hasToolResult = true
			if s := extractToolResult(b["content"]); s != "" {
				parts = append(parts, s)
			}
		}
	}
	text = strings.Join(parts, "\n")
	return text, text != "", hasToolUse, hasToolResult
}

// extractToolResult reads a tool_result's content, which may be a string or an
// array of text blocks.
func extractToolResult(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		return str
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		var s string
		if json.Unmarshal(b["text"], &s) == nil && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

func parseTimestamp(s string) *time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}
