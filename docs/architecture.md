---
phase: architecture
status: approved
approved: 2026-07-17
rules: ./arch-rules.yaml
date: 2026-07-16
---
<!-- CONTRACT: Module boundaries and WHY they sit where they do. The
     machine-readable source of truth is arch-rules.yaml — this doc must
     agree with it exactly. Entity shapes live in data-models.md. -->

# Sutra — Architecture

## Style
Layered, with a client/server split. vision.md's shape is one daemon on the
LAN serving a TUI and CLI from every machine, so the modules divide into a
**server side** (`store` → `service` → `api`) that owns the data and a
**client side** (`client` → `tui`/`cli`) that talks to it over HTTP. Both
sides share `domain` (types) and `config`. The TUI and CLI always reach the
daemon through `client` over HTTP — even on the host against localhost — so
the daemon stays the single source of truth (vision.md success criteria).
The composition root is `cmd/sutra/main.go`, outside `internal/`, so it is
not governed by these rules.

## Modules
| Module | Purpose | May import |
|--------|---------|------------|
| `domain` | Core entities (Issue, IssueType, Document, Transcript, and their links) and rules. Pure. | nothing |
| `config` | Load and validate configuration from `~/.config/sutra/config.toml`: DB path, listen address, host, bearer token. | nothing |
| `store` | SQLite + FTS5 persistence implementing the domain repositories. | `domain`, `config` |
| `service` | Use-cases: create an issue, attach a DDD document, ingest and link a transcript, search. Opens the store. | `domain`, `store`, `config` |
| `api` | HTTP daemon — routes and handlers over `service`; `api.Run` wires the service and serves (`sutra serve`). | `domain`, `service`, `config` |
| `client` | HTTP client to the daemon; returns domain types. | `domain`, `config` |
| `tui` | Bubble Tea terminal UI over `client`. | `domain`, `client` |
| `cli` | Cobra commands: `serve` uses `api`; all other commands use `client`. | `domain`, `client`, `api`, `config` |

Flow: `cli`/`tui` → `client` → HTTP → `api` → `service` → `store` → SQLite.

## Rationale
- **`domain` and `config` are leaves** — everything depends inward on shared
  types and configuration, nothing depends back out, so they stay stable.
- **The server chain is one-directional** (`store` → `service` → `api`):
  persistence knows nothing of use-cases, use-cases know nothing of HTTP.
  Changing the transport (add TLS, swap routing) touches only `api`;
  changing the schema touches only `store` and the entities in `domain`.
- **`client` depends only on `domain` + `config`**, never on server modules,
  so the client side cannot reach into `store`/`service` and bypass the
  daemon — the HTTP boundary is enforced by the import graph.
- **`cli` may import `api`** solely so the `serve` command can start the
  daemon; every other command goes through `client`. This keeps the single
  binary able to be both server and client without a cycle.

## Enforcement
Rules are codified in [arch-rules.yaml](./arch-rules.yaml) and generated
into architecture tests at scaffold time (see docs/scaffold.md once
scaffolded). The YAML is the source of truth; regenerate tests rather than
hand-editing them.

**The .md and .yaml must agree exactly** — same module names, same
may_import lists. The architecture-reviewer diffs them. `may_import` is the
source of truth (a dependency you don't want is simply left out of it);
`rules:` holds only `no_cycles`.
