# Claude Context — Sutra

Guidance for Claude Code sessions working in this repository. For user-facing
usage, see [`README.md`](README.md).

## What this is

Sutra is a single-user, self-hosted **issue tracker for agent-assisted
development**. Feature issues carry their doc-driven-development documents
(problem / design / plan / scenarios) and link the Claude transcripts that
produced them; everything is full-text searchable in one place.

One Go binary, three faces: `sutra serve` (daemon), `sutra` (TUI),
`sutra <verb>` (CLI). The CLI/TUI are thin clients over the daemon's HTTP API.

## Stack

- Go 1.26, single static binary (no cgo).
- **Cobra** CLI, **Bubble Tea** (Charm) TUI, stdlib **net/http** daemon.
- **SQLite** via `modernc.org/sqlite` (pure Go), **FTS5** full-text search,
  WAL + `busy_timeout`.

## Architecture — module boundaries are enforced

Packages live under `internal/<module>`. Imports between them are restricted by
`architecture_test.go` (reads `docs/arch-rules.yaml` as the source of truth via
`go list -json ./internal/...`). Allowed imports:

| module    | may import                     |
| --------- | ------------------------------ |
| `domain`  | *(nothing)*                    |
| `config`  | *(nothing)*                    |
| `store`   | domain, config                 |
| `service` | domain, store, config          |
| `api`     | domain, service, config        |
| `client`  | domain, config                 |
| `tui`     | domain, client                 |
| `cli`     | domain, client, api, config    |

Plus `no_cycles`. The composition root is `cmd/sutra/main.go` — it wires the
service, api, client, and injects `cli.TUILauncher`. **If you add a cross-module
import, update `arch-rules.yaml` deliberately — don't work around the test.**

Request flow: `cli`/`tui` → `client` → HTTP → `api` → `service` → `store` → SQLite.
`domain` holds pure types (entities, enums with `.Valid()`, filters, errors).

## Development workflow — non-negotiable

1. **BDD-first.** Every slice is driven by godog scenarios in `features/`.
   Write/enable the scenario, make it go red→green.
   - `bdd_test.go` holds `const implementedTags` (e.g. `"@slice1,@slice2,..."`).
     **godog's OR operator is a comma, not `||`** — `||` matches zero scenarios
     and passes vacuously. Always use commas.
   - `TestImplemented` runs shipped slices; `TestBacklog` (opt-in via
     `SUTRA_BACKLOG=1`) runs the rest.
2. **roborev gate.** A slice is not done until `roborev` returns a Pass (P).
   Use `/roborev-refine` (review→fix→re-review loop) with `--since <base-commit>`;
   `--branch` fails here (can't detect the default branch).
3. **Verify before claiming done.** Run `go build ./...`, `go vet ./...`,
   `go test -race ./...`, and confirm the scenario count. State what you ran.

## Commands

```bash
go build -o sutra ./cmd/sutra
go test ./...                                 # architecture rules + implemented BDD
go test -count=1 -run TestImplemented ./...   # BDD for shipped slices
go test -race ./...                           # full suite under the race detector
SUTRA_BACKLOG=1 go test -run TestBacklog ./...
```

## Conventions & invariants

- **Config is a TOML file**, not env vars. `config.Load()` reads
  `$XDG_CONFIG_HOME/sutra/config.toml` (default `~/.config/sutra/config.toml`),
  overlaying set keys onto defaults; a missing file uses defaults, an unknown key
  or malformed file is a startup error. Keys: `listen_addr`, `host`, `token`,
  `db_path`, `projects_dir`. (`SUTRA_BACKLOG` is a test-only toggle, unrelated.)
- **Ledger on every mutation.** Every issue-changing operation writes an
  append-only `LedgerEntry` and bumps `updated_at`, in the same transaction.
  Ledger entries are never updated or deleted.
- **Soft delete.** Issues carry a `deleted` flag; `list` excludes them.
- **Reads are snapshots.** `GetIssue`/`ListIssues` run inside a read transaction
  and hydrate labels/links via seams (`afterIssueReadHook`).
- **Transcripts:** one entry per Claude `.jsonl` session, split into ordered
  `Message`s; `raw` (original JSON line) is kept as the lossless render source.
  Ingest/discover is confined to the configured `projects_dir` — path traversal
  outside it is rejected (TOCTOU-safe via pinned parent root).
- **Auth (LAN).** Bearer token; constant-time compare over SHA-256 digests;
  scheme is case-insensitive; the daemon refuses to bind a non-loopback address
  without a `token` set in config. Server sets `ReadHeaderTimeout`/`IdleTimeout`.
- **Terminal safety.** All daemon-provided strings are sanitized (`clean`/
  `cleanLine` in `cli`/`tui` `safe.go`) to strip ANSI/control sequences before
  display; TUI truncation is ANSI-display-width aware.
- **`--json` is verbatim passthrough** — `printRawJSON` writes the daemon's bytes
  byte-for-byte, appending a single trailing newline only when absent. Don't add
  trimming or reformatting.
- **Enums** validate at the API boundary (invalid → 400): status
  `open|in_progress|closed`, type `feature|bug|task|chore`, priority `p0..p3`,
  doc kind `problem|design|plan|scenarios`, search kind `issue|document|message`.
- File perms: DB dir `0700`, DB files `0600`.

## Docs

`docs/`: `vision.md`, `stack.md`, `architecture.md` (+ `arch-rules.yaml`),
`data-models.md`, `STORIES.md`, `PLAN.md`, `scaffold.md`. `PLAN.md` defines the
slices; `data-models.md` is the schema source of truth.

## Status

Slices 1–8 are implemented, individually roborev-passed, and merged to
`develop`. Built on top since: slice 9 (`activity` feed), slice 10 (`project`
entity — 1 project = 1 repo; nullable `project_id` on issues/threads;
transcripts auto-associate by matching their session cwd to `repo_path`), and
slice 11 (`thread` meta-object grouping issues/documents/transcripts/comments
many-to-many). Not yet done: deploy to the LAN server box.
