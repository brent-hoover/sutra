Feature: Transcript capture

  Scenario: Ingest a transcript
    Given a session file under ~/.claude/projects
    When I ingest it
    Then a Transcript is stored with session_id, source_path, captured_at, and a title derived from the first user message
    Given that file's lines
    When ingestion runs
    Then each line becomes a Message with seq, role, extracted text, original raw, and at when present

  Scenario: Re-ingest is idempotent
    Given a transcript already ingested
    When I ingest the same session_id again
    Then the existing Transcript and its Message rows are updated in place, not duplicated

  Scenario: Link a transcript to an issue
    Given an ingested transcript and an issue
    When I link them
    Then the transcript's issue_id is set and a LedgerEntry of kind linked is appended to the issue

  Scenario: View an issue's transcripts
    Given an issue with linked transcripts
    When I view its transcripts
    Then each linked Transcript appears with its title and captured_at

  Scenario: Read a transcript
    Given an ingested transcript
    When I open it
    Then its Message rows render in seq order by role, reconstructed from raw

  Scenario: Discover local transcripts
    Given session files on disk
    When I list available transcripts, optionally by project dir
    Then each file's session_id, path, and ingested state is shown
