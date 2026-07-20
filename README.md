# Sutra

An issue tracker for agent-assisted development. Feature issues carry their
doc-driven-development documents (problem / design / plan / scenarios) and link
the Claude transcripts that produced them — issues, docs, and transcripts are
all full-text searchable in one place. Single user, self-hosted, local-first.

Delivered as **one Go binary** with three faces:

- `sutra serve` — the daemon (HTTP API over SQLite)
- `sutra` — the interactive TUI
- `sutra <verb>` — the CLI (a thin JSON client over the daemon)

## Install

Requires Go 1.26+. No cgo — SQLite is pure Go (`modernc.org/sqlite`).

```bash
go build -o sutra ./cmd/sutra
# or install onto your PATH:
go install github.com/brent-hoover/sutra/cmd/sutra@latest
```

## Quickstart

```bash
# 1. Start the daemon (foreground; loopback by default)
sutra serve

# 2. In another shell — create and inspect an issue
sutra create --subject "Wire up search" --body "FTS across issues + docs"
sutra list
sutra view <id>

# 3. Or just launch the TUI (no subcommand)
sutra
```

The CLI and TUI both talk to the daemon over HTTP — start `sutra serve` first.
Add `--json` to any CLI command to print the daemon's raw JSON response verbatim
(useful for scripting and for agents).

## Configuration

Configuration is read from a TOML file at
**`~/.config/sutra/config.toml`** (or `$XDG_CONFIG_HOME/sutra/config.toml`).
A missing file uses the defaults below; any key you omit falls back to its
default. An unknown key or malformed file is a startup error.

```toml
# ~/.config/sutra/config.toml
listen_addr  = "127.0.0.1:8422"       # daemon bind address (serve)
host         = "http://127.0.0.1:8422" # endpoint the CLI/TUI target
token        = ""                       # bearer token; required to bind non-loopback
db_path      = "~/.sutra/sutra.db"      # SQLite database file
projects_dir = "~/.claude/projects"     # dir transcript ingest/discover is confined to
```

| Key            | Default                     | Purpose                                                |
| -------------- | --------------------------- | ------------------------------------------------------ |
| `listen_addr`  | `127.0.0.1:8422`            | Daemon bind address (`serve`)                          |
| `host`         | `http://127.0.0.1:8422`     | Endpoint the CLI/TUI target                            |
| `token`        | *(empty)*                   | Bearer token; required to bind a non-loopback address  |
| `db_path`      | `~/.sutra/sutra.db`         | SQLite database file                                   |
| `projects_dir` | `~/.claude/projects`        | Directory transcript ingest/discover is confined to    |
| `skills_dir`   | `~/.claude/skills`          | Default directory `skill install` writes skills into    |

> The `token` is a secret in plaintext. When the file sets a `token`, Sutra
> **refuses to start unless the file is owner-only** (`chmod 600
> ~/.config/sutra/config.toml`), the same posture as SSH and Postgres.

## Command reference

Global flag: `--json` (raw JSON response). IDs are the values printed by `create`
and `list`.

### Issues

```bash
sutra create --subject <s> --body <b>          # only subject + body are required
sutra list [--status <s>] [--type <t>] [--priority <p>] [--owner <o>] [--label <l>]
sutra view <id>                                # fields, labels, links, comments
sutra update <id> [--status open|in_progress|closed] \
                  [--type feature|bug|task|chore] \
                  [--priority p0|p1|p2|p3] [--owner <agent>]
sutra delete <id>                              # soft delete
sutra history <id>                             # ledger of every change
sutra comment <id> --body <b> [--author <a>]
```

### Links & labels

```bash
sutra link parent  <id> --parent <parent-id>
sutra link related <id> --to <other-id>        # symmetric
sutra link blocked <id> --by <blocker-id>
sutra label add    <id> <label>
sutra label remove <id> <label>
```

### Documents

Kinds: `problem | design | plan | scenarios`.

```bash
sutra doc attach <issue-id> --kind <k> --content <md> [--title <t>]
sutra doc list   <issue-id>
sutra doc read   <doc-id>
sutra doc update <doc-id> --content <md>
sutra doc remove <doc-id>
```

### Transcripts

Capture Claude `.jsonl` session files (one entry per session, broken into
messages). Ingest is confined to the configured `projects_dir`.

```bash
sutra transcript discover                       # local sessions + ingested state
sutra transcript ingest <path>
sutra transcript view <id>                      # messages in order
sutra transcript link <transcript-id> --issue <issue-id>
sutra transcript list <issue-id>
```

### Search

Full-text across issues, documents, and transcript messages (SQLite FTS5).

```bash
sutra search <query> [--kind issue|document|message] [--issue <id>] [--limit <n>]
```

### Skills

Store reusable agent skills (a SKILL.md) and install them to a machine's skills
directory. `install` is client-side: it fetches the skill from the daemon and
writes it locally to `<skills_dir>/<slug>/SKILL.md`.

```bash
sutra skill create --name "Graphify" --content-file ./SKILL.md   # or --content "..."
sutra skill list
sutra skill view <id>
sutra skill update <id> --content-file ./SKILL.md
sutra skill install <id>              # → ~/.claude/skills/<slug>/SKILL.md (or skills_dir)
sutra skill install <id> --dir ./.claude/skills
sutra skill delete <id>
```

### Activity ("what was I working on?")

A reverse-chronological feed across issue changes and captured Claude
transcripts, scoped to a time window — the Monday-morning catch-up view. It
**auto-ingests** any Claude session modified within the window first, so recent
work shows up even if you never ingested it manually.

```bash
sutra activity                       # last 3 days (default)
sutra activity --since 24h           # a Go duration
sutra activity --since 3d            # 'd' = days
sutra activity --since 2026-07-17    # or a calendar date
```

## Running on your LAN

To reach the daemon from your other machines, bind a non-loopback address. A
bearer token is **required** in that case — Sutra refuses to serve unauthenticated
beyond loopback.

On the server (e.g. the Ubuntu box) — `~/.config/sutra/config.toml`:

```toml
listen_addr = "0.0.0.0:8422"
token       = "<a long random secret, e.g. from `openssl rand -hex 32`>"
```

On each client machine — `~/.config/sutra/config.toml`:

```toml
host  = "http://<server-ip>:8422"
token = "<same token>"
```

Then `sutra serve` on the server and `sutra list` on a client.

Auth is bearer-only over the wire; run it on a trusted network (there is no TLS
termination built in — front it with a reverse proxy if you need HTTPS).

## Development

Every slice is built BDD-first with [godog](https://github.com/cucumber/godog);
package boundaries are enforced by an architecture test.

```bash
go test ./...                                   # architecture rules + implemented BDD
go test -count=1 -run TestImplemented ./...     # BDD for shipped slices (@slice* tags)
go test -race ./...                             # full suite under the race detector
SUTRA_BACKLOG=1 go test -run TestBacklog ./...  # opt-in: scenarios not yet implemented
```

## Docs

Planning docs live in [`docs/`](docs/): vision, stack, architecture (+
`arch-rules.yaml`), data-models, STORIES, PLAN. See [`CLAUDE.md`](CLAUDE.md) for
the architecture map and conventions.
