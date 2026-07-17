---
phase: stack
status: approved
approved: 2026-07-16
language: go
date: 2026-07-16
---
<!-- CONTRACT: Records stack decisions ADR-style, one entry per dimension,
     each with alternatives and rationale. The `language` frontmatter field
     gates the scaffold phase. No module boundaries — that's architecture.md. -->

# Sutra — Stack

## Language
- **Decision:** Go (1.26).
- **Alternatives considered:**
  - Python — the owner's strongest language, but shipping a daemon + TUI +
    CLI across several machines needs a runtime and deps on each; heavier
    than a single binary.
  - Rust — also single-binary, but steeper for the owner and no stated
    interest.
- **Rationale:** vision.md's shape (one daemon on a home-network box, TUI and
  CLI used from all the owner's machines) is best served by a single static
  binary with trivial cross-compilation — Go's core strength. kata
  (https://github.com/kenn-io/kata), the tool Sutra is modeled on, is Go with
  this same CLI + daemon + TUI + SQLite shape, so its patterns are reference
  material. Also advances the owner's Go skills.

## Framework
- **Decision:** one executable, three surfaces selected by invocation — like
  kata. `sutra serve` runs the daemon, `sutra` (no subcommand) launches the
  TUI, `sutra <verb> …` runs a CLI command. Same binary everywhere.
  - CLI: Cobra (defines all subcommands, including `serve`).
  - TUI: Bubble Tea + Bubbles + Lipgloss (Charm).
  - Daemon/API: Go stdlib `net/http` with the 1.22+ `ServeMux` routing.
- **Alternatives considered:**
  - Gin/Echo/Chi for the daemon — extra dependency for routing/middleware
    not needed at single-user scale; stdlib now covers path params.
  - urfave/cli instead of Cobra — Cobra is the more common Go CLI framework;
    broader ecosystem and examples.
  - tview instead of Bubble Tea — Bubble Tea is the modern Charm standard.
- **Rationale:** vision.md's single-user LAN daemon needs no web framework —
  stdlib `net/http` keeps the binary lean and dependency-light. Cobra + Charm
  are the idiomatic Go CLI/TUI stack; because kata is built the same way, its
  command and TUI structure serve as a working reference. Sutra is built
  fresh (not forked), but may lift specific MIT-licensed pieces of kata —
  e.g. TUI components — with license/attribution preserved.

## Storage
- **Decision:** SQLite (single file) with FTS5 full-text search, via the
  pure-Go `modernc.org/sqlite` driver. Document and transcript *content* is
  stored in the DB, not as file-path references.
- **Alternatives considered:**
  - `mattn/go-sqlite3` — most popular, but cgo requires a C toolchain and
    complicates cross-compilation; breaks the clean static-binary story.
  - Files-on-disk for docs/transcripts + SQLite for metadata — files live on
    one machine, breaking LAN-wide access and single-source-of-truth.
  - Postgres or an embedded server DB — overkill for single-user; adds an
    operational dependency.
- **Rationale:** one SQLite file on the daemon host holds issues, DDD
  documents, and transcripts as the single source of truth all clients read.
  FTS5 delivers success-criterion 4's cross-entity search with no external
  engine. `modernc.org/sqlite` (no cgo) preserves the single static binary
  and easy cross-compiles, and bundles FTS5.

## Packaging & tooling
- **Decision:** Go modules (Go 1.26); GoReleaser for cross-platform builds
  and distribution (binaries, `.deb`/`.rpm`, checksums); a `Makefile`
  (`build`, `test`, `lint`, `run`); golangci-lint + gofmt/goimports; tests
  via stdlib `go test` + testify, with **godog** (Cucumber for Go) running
  the BDD scenarios; CI on GitHub Actions (lint + test, GoReleaser on tag).
- **Alternatives considered:**
  - `just` instead of Make — nicer syntax but an extra install; Make is
    everywhere.
  - Plain `go test` only, skipping godog — the project is BDD/doc-driven, so
    the scaffold-generated Gherkin scenarios need godog to be executable.
  - Hand-rolled release scripts instead of GoReleaser — GoReleaser is the Go
    standard and gives kata-parity distribution for free.
- **Rationale:** the daemon + TUI + CLI must be installed on multiple
  machines (vision.md's shape), so GoReleaser's cross-platform artifacts and
  per-OS packages are what make that practical from one build. The owner's
  doc-driven/BDD flow (vision.md Problem) is the reason godog is included —
  it makes the scaffold-generated Gherkin specs executable. Remaining choices
  (Make, golangci-lint, testify) are the standard Go toolchain.

## Deployment target
- **Decision:** the same single `sutra` binary installs on every machine. On
  the Ubuntu home-server box a `systemd` service runs `sutra serve`, bound to
  the LAN address. On each macOS client the same binary runs the TUI/CLI
  subcommands, pointing at the daemon via config / `SUTRA_HOST`. (The Ubuntu
  host and macOS clients are specified by the owner; vision.md states only
  "one home-network machine" and "all their computers".)
  Transport is plain HTTP over the home LAN with a shared bearer token in
  client config. Artifacts installed per machine via GoReleaser output.
- **Alternatives considered:**
  - Docker container for the daemon — clean, but adds a runtime dependency on
    the host; a single binary + systemd is lighter for a personal tool.
  - TLS from day one — more secure but more setup; deferrable on a trusted
    LAN.
- **Rationale:** systemd fits the Ubuntu host; a bearer token over LAN HTTP
  is enough for a single user on a trusted network, with TLS available later.
