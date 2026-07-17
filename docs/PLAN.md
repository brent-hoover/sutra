---
phase: plan
status: approved
approved: 2026-07-17
date: 2026-07-17
---
<!-- CONTRACT: Ordered milestone slices over the stories in STORIES.md.
     Each slice ships working software. Story content lives in STORIES.md —
     reference titles, don't restate criteria. -->

# Sutra — Plan

## Method

Each slice is implemented BDD-first. The scaffold (phase 7) generates godog
`.feature` files from STORIES.md that start red. A slice is done when its
stories' scenarios pass (red → green) **and** its **Verify** line — the
end-to-end sanity check — holds. STORIES.md acceptance criteria are the
source of truth for the scenarios; the Verify lines below are the manual
confirmation on top.

**Cross-cutting stories.** A few stories span multiple slices and are not
completed by one:

- *View an issue* (full, with comments/labels/links) needs entities from S3
  (comments) and S4 (labels/links). Its slice-1 increment is the separate
  *View an issue's core fields* story; the full story completes in S4.
- *JSON API covers every operation* and *CLI is a thin client* are evergreen
  invariants, not one-time completions: each slice that adds an operation
  must add its JSON endpoint and thin CLI command, re-checked per slice for
  the operations present at that point.
- *Manage issues in the TUI* (S8) grows as the operations it drives land in
  earlier slices; its scenario is verified against the capabilities present
  when S8 is built.

Scenarios are tagged by slice (`@slice1`, …). The default test run
(`TestImplemented`) executes only implemented tags and stays green; the full
red backlog is opt-in (`SUTRA_BACKLOG=1`, `TestBacklog`).

## Dependency graph

```
S1 Skeleton ─▶ S2 Transport contract ─┬─▶ S3 Issue CRUD ─▶ S4 Linking & labels ─┐
                                       ├─▶ S5 Documents ───────────────────────┼─▶ S7 Search
                                       └─▶ S6 Transcripts ─────────────────────┘
                                                                                └─▶ S8 TUI (after S3)
```

- **S1 → S2 are sequential and first** — they stand up the spine and the
  client/server contract every later slice builds against.
- **After S2, three tracks run in parallel:** S3 (issues), S5 (documents),
  S6 (transcripts) touch different entities and endpoints. S4 follows S3.
- **S7 (search) is the fan-in** — it needs issues, documents, and messages
  to exist.
- **S8 (TUI)** can start once S3 lands and grow as the other tracks do.

## Parallelization constraint

To let multiple agents work concurrently after S2 without colliding:

- **Per-feature files within each module** — `store/issues.go`,
  `store/documents.go`, `store/transcripts.go`, and likewise in `service`
  and `api`. A feature slice edits its own files, not a shared one.
- **Thin registration points** — one small place per module (e.g. route
  registration in `api`, migration list in `store`) that each slice appends
  to. Keep these append-only and short to minimize merge conflicts.
- Each parallel slice is developed in its own git worktree/branch and
  verified independently before merge.

## Slice 1: Walking skeleton
- **Stories:** Run the daemon · Create an issue · View an issue's core fields
- **Why now:** stands up the full `cli → client → HTTP → api → service →
  store → SQLite` spine with all 8 modules touched minimally, and sets the
  JSON-endpoint + thin-CLI pattern every later slice follows. Localhost, no
  auth yet.
- **Verify:** `sutra serve &`, then `sutra create --subject x --body y`
  followed by `sutra view <id>` returns the issue as JSON.

## Slice 2: Transport contract & hardening
- **Stories:** JSON API covers every operation · CLI is a thin client ·
  Authenticated LAN access · Use clients from another machine
- **Why now:** locks the real client/server contract (config, bearer token,
  `SUTRA_HOST`) directly after the skeleton, before features accrete.
- **Verify:** a request with a wrong/missing token returns 401; with the
  correct token, 200; the CLI pointed at `SUTRA_HOST` from another machine
  operates on the daemon's data.

## Slice 3: Issue CRUD complete
- **Stories:** List issues · Update issue fields · Comment on an issue ·
  Soft-delete an issue · Filter issues · View an issue's change history
- **Why now:** completes the core issue loop and the change ledger on the
  skeleton. Head of the issues track.
- **Verify:** create several issues, update/comment/delete them,
  `sutra list --status open --type bug` returns the expected set (filters use
  only S3-supported fields; label filtering arrives with labels in S4), and
  issue history shows the `LedgerEntry` rows.

## Slice 4: Issue linking & labels
- **Stories:** Link issues · Label an issue · View an issue
- **Why now:** richer issue structure once core CRUD exists (follows S3).
  The full *View an issue* story completes here — it is the first slice where
  comments (S3), labels, and links all exist.
- **Verify:** set parent/related/blocking links and labels via CLI; the
  issue view shows them; a full `sutra view <id>` shows comments, labels, and
  links.

## Slice 5: Documents
- **Stories:** Attach a document to an issue · Read a document · List an
  issue's documents · Update a document · Remove a document
- **Why now:** documents hang off issues, which exist after S1. Independent
  track — parallel with S3/S6.
- **Verify:** attach, list, read, update, and remove a document on an issue
  via CLI.

## Slice 6: Transcript capture
- **Stories:** Ingest a transcript · Re-ingest is idempotent · Link a
  transcript to an issue · View an issue's transcripts · Read a transcript ·
  Discover local transcripts
- **Why now:** transcripts and their messages plus issue links. Independent
  track — parallel with S3/S5.
- **Verify:** ingest a real `~/.claude/projects/**/*.jsonl`; re-ingest the
  same session produces no duplicate; link it to an issue; view the issue's
  transcripts.

## Slice 7: Search
- **Stories:** Full-text search across everything · Message hits carry
  context · Ranked results · Scoped search
- **Why now:** search spans issues, documents, and messages, so it is the
  fan-in after S3, S5, and S6.
- **Verify:** seed a known term in an issue, a document, and a message; one
  search returns all three with surrounding context; results are ranked; a
  scoped search by `kind`/`issue` narrows correctly.

## Slice 8: TUI
- **Stories:** Manage issues in the TUI
- **Why now:** interactive front-end once the API surface is stable; can
  begin after S3 and extend as later tracks land.
- **Verify:** launch the TUI, create/edit/close an issue and create a child;
  open an issue to see its documents, comments, and linked transcripts.
