---
phase: stories
status: approved
approved: 2026-07-17
date: 2026-07-17
---
<!-- CONTRACT: The v1 backlog. Story → Given/When/Then acceptance criteria,
     the same shape core:user-stories produces and bdd-specs consumes.
     Ordering/slicing lives in PLAN.md. -->

# Sutra — Stories

## Epic: Issue tracking

### Story: Create an issue
As a developer, I want to create an issue with a subject and body so that work is captured.

**Acceptance criteria:**
- Given no other input, When I create an issue with a subject and body, Then it is stored with a generated `id` and defaults `type=task`, `status=open`, `priority=p2`, and timestamps set
- Given a new issue is created, When it is stored, Then a `LedgerEntry` of kind `created` is appended
- Given a missing subject or body, When I try to create the issue, Then it is rejected

**Priority:** must

### Story: List issues
As a developer, I want to list issues so that I can see open work.

**Acceptance criteria:**
- Given several live issues exist, When I list issues, Then all live issues are returned
- Given one of those issues is soft-deleted, When I list issues, Then the soft-deleted one is excluded by default

**Priority:** must

### Story: View an issue
As a developer, I want to view a single issue with its comments and links so that I have full context.

**Acceptance criteria:**
- Given an issue with comments, labels, and links, When I view it by `id`, Then its fields, `labels`, related/blocking links, and comments are shown

**Priority:** must

### Story: Update issue fields
As a developer, I want to change an issue's type, status, priority, or owner so that it reflects reality.

**Acceptance criteria:**
- Given an open issue, When I change its `status` to `in_progress`, Then the field updates and `updated_at` advances
- Given a field on an issue changes, When the change is saved, Then a `LedgerEntry` (kind `status_changed`, with `field`, `old_value`, `new_value`) is appended

**Priority:** must

### Story: Comment on an issue
As a developer or agent, I want to add comments so that discussion stays with the issue.

**Acceptance criteria:**
- Given an issue, When I add a comment with a body, Then a `Comment` is stored and a `LedgerEntry` of kind `commented` is appended

**Priority:** must

### Story: Soft-delete an issue
As a developer, I want to delete an issue without losing history so that mistakes are recoverable.

**Acceptance criteria:**
- Given an issue, When I delete it, Then `deleted_at` is set and it drops from default lists and search
- Given an issue is deleted, When the change is saved, Then a `LedgerEntry` of kind `deleted` is appended

**Priority:** must

### Story: Link issues
As a developer, I want to link issues by hierarchy, relation, and blocking so that dependencies are explicit.

**Acceptance criteria:**
- Given two issues, When I set one as another's `parent_id`, Then the child/parent relation holds
- Given an issue, When I set its `parent_id` to itself, Then it is rejected
- Given two issues, When I mark A as blocked by B, Then A shows B in `blocked_by` and B shows A in `is_blocking` (derived from `issue_block`)
- Given two issues, When I relate them, Then each appears in the other's related list

**Priority:** should

### Story: Label an issue
As a developer, I want to add and remove labels so that I can categorize issues.

**Acceptance criteria:**
- Given an issue, When I add a label, Then it appears in `labels`
- Given an issue with a label, When I remove the label, Then it no longer appears in `labels`

**Priority:** should

### Story: View an issue's change history
As a developer, I want to see an issue's ledger so that I can audit what changed.

**Acceptance criteria:**
- Given an issue with several changes, When I view its history, Then `LedgerEntry` rows appear in chronological order

**Priority:** should

### Story: Filter issues
As a developer, I want to filter the issue list by field so that I can find relevant work fast.

**Acceptance criteria:**
- Given issues with varied `status`, `type`, `priority`, `labels`, and `owner`, When I list with a filter (e.g. `status=open`, `label=bug`, `owner=<agent>`), Then only matching, non-soft-deleted issues are returned
- Given multiple filters, When I combine them, Then results match all filters (AND)

**Priority:** must

## Epic: Documents

### Story: Attach a document to an issue
As a developer, I want to attach a DDD document to an issue so that its planning artifacts live with the work.

**Acceptance criteria:**
- Given any issue, When I attach a `Document` with a `kind` (`problem`|`design`|`plan`|`scenarios`) and markdown `content`, Then it is stored linked to that issue with timestamps set
- Given empty `content`, When I try to attach, Then it is rejected

**Priority:** must

### Story: Read a document
As a developer, I want to read a document so that I can review the planning artifact.

**Acceptance criteria:**
- Given an issue with a document, When I open the document by `id`, Then its `kind`, `title`, and `content` are returned

**Priority:** must

### Story: List an issue's documents
As a developer, I want to see all documents on an issue so that I can navigate its artifacts.

**Acceptance criteria:**
- Given an issue with several documents, When I list its documents, Then all appear with their `kind` and `title`

**Priority:** must

### Story: Update a document
As a developer, I want to update a document's content so that artifacts stay current.

**Acceptance criteria:**
- Given an existing document, When I update its `content`, Then the new content is stored and `updated_at` advances

**Priority:** should

### Story: Remove a document
As a developer, I want to remove a document so that stale artifacts can be cleared.

**Acceptance criteria:**
- Given an issue with a document, When I remove it, Then it no longer appears in the issue's document list

**Priority:** could

## Epic: Transcript capture

### Story: Ingest a transcript
As a developer, I want to ingest a Claude `.jsonl` session file so that the conversation is captured.

**Acceptance criteria:**
- Given a session file at `~/.claude/projects/<encoded-cwd>/<uuid>.jsonl`, When I ingest it, Then a `Transcript` is stored with `session_id`, `source_path`, `captured_at`, and a `title` derived from the first user message
- Given that file's lines, When ingestion runs, Then each line becomes a `Message` with `seq`, `role`, extracted `text`, original `raw`, and `at` when present

**Priority:** must

### Story: Re-ingest is idempotent
As a developer, I want re-ingesting a session to update rather than duplicate so that captures stay clean.

**Acceptance criteria:**
- Given a transcript already ingested, When I ingest the same `session_id` again, Then the existing `Transcript` and its `Message` rows are updated in place, not duplicated

**Priority:** must

### Story: Link a transcript to an issue
As a developer, I want to link a transcript to an issue so that the conversation lives with the work.

**Acceptance criteria:**
- Given an ingested transcript and an issue, When I link them, Then the transcript's `issue_id` is set and a `LedgerEntry` of kind `linked` is appended to the issue

**Priority:** must

### Story: View an issue's transcripts
As a developer, I want to see the transcripts linked to an issue so that I can find the conversation that produced the work.

**Acceptance criteria:**
- Given an issue with linked transcripts, When I view its transcripts, Then each linked `Transcript` appears with its `title` and `captured_at`

**Priority:** must

### Story: Read a transcript
As a developer, I want to read a transcript's messages in order so that I can revisit the thread.

**Acceptance criteria:**
- Given an ingested transcript, When I open it, Then its `Message` rows render in `seq` order by `role`, reconstructed from `raw`

**Priority:** should

### Story: Discover local transcripts
As a developer, I want to list uningested session files under `~/.claude/projects` so that I can pick what to capture.

**Acceptance criteria:**
- Given session files on disk, When I list available transcripts (optionally by project dir), Then each file's `session_id`, path, and ingested/not-ingested state is shown

**Priority:** could

## Epic: Search

### Story: Full-text search across everything
As a developer or agent, I want to search issues, documents, and transcript messages with one query so that I can find any past work.

**Acceptance criteria:**
- Given a known term present in an `Issue.subject`/`body`, a `Document.content`, and a `Message.text`, When I search for that term, Then hits from all three appear
- Given soft-deleted issues, When I search, Then their content is excluded

**Priority:** must

### Story: Message hits carry context
As an agent, I want each transcript hit to include surrounding messages so that I can synthesize an answer without re-fetching the whole session.

**Acceptance criteria:**
- Given a matching `Message`, When it is returned, Then it includes its `role`, `text`, `seq`, and its owning `Transcript` (and linked `Issue` if any)
- Given a matching `Message`, When it is returned, Then it includes a window of adjacent messages for context

**Priority:** must

### Story: Ranked results
As a developer, I want results ordered by relevance so that the best matches come first.

**Acceptance criteria:**
- Given multiple matches, When I search, Then results are ranked by relevance and each shows its source kind (issue/document/message)

**Priority:** should

### Story: Scoped search
As a developer, I want to restrict search to a kind or a single issue so that I can narrow results.

**Acceptance criteria:**
- Given a scope (e.g. `kind=document`, or `issue=<id>`), When I search within it, Then only matches in that scope are returned

**Priority:** could

## Epic: Daemon & clients

### Story: Run the daemon
As a developer, I want to start the daemon so that clients have a shared service.

**Acceptance criteria:**
- Given a config with a listen address and DB path, When I run `sutra serve`, Then it opens the store and serves HTTP on that address

**Priority:** must

### Story: JSON API covers every operation
As an agent, I want every capability exposed over a JSON HTTP API so that automation needs no other entry point.

**Acceptance criteria:**
- Given the running daemon, When any operation from the other epics is invoked over HTTP, Then it accepts and returns JSON, one endpoint per operation

**Priority:** must

### Story: CLI is a thin client
As a developer, I want the CLI to map directly to the API so that it's predictable and stays logic-free.

**Acceptance criteria:**
- Given the daemon is running, When I run a CLI command, Then it calls exactly one API endpoint and holds no behavior the API doesn't expose
- Given `--json`, When I run a CLI command, Then it prints the API's raw JSON response

**Priority:** must

### Story: Authenticated LAN access
As a developer, I want clients to authenticate so that only my machines reach the daemon.

**Acceptance criteria:**
- Given a configured bearer token, When a client sends a request with the correct token, Then it succeeds
- Given a wrong or missing token, When a client sends a request, Then it is rejected with 401

**Priority:** must

### Story: Use clients from another machine
As a developer, I want to point a client at the daemon over the LAN so that I can work from any of my computers.

**Acceptance criteria:**
- Given `SUTRA_HOST` set to the daemon's LAN address and a valid token, When I run the CLI or TUI from another machine, Then it operates against the daemon's data

**Priority:** must

### Story: Manage issues in the TUI
As a developer, I want to create, edit, and close issues and create children from the TUI so that I can do real work interactively, not just browse.

**Acceptance criteria:**
- Given the TUI is open, When I create an issue, Then it is added (same rules as the create story) and appears in the list
- Given an open issue, When I edit its fields or change its status to `closed`, Then the change persists and the ledger records it
- Given an issue, When I create a child from it, Then the new issue's `parent_id` is set to it
- Given an issue, When I view it, Then I can see its documents, comments, and linked transcripts

**Priority:** should
