---
title: Plan Tickets — Problem Statement
type: problem
status: draft
owner: Brent Hoover
created: 2026-07-20
updated: 2026-07-20
---

# Plan Tickets — Problem Statement

## Context

Sutra tracks agent-assisted, doc-driven development. Feature work is decomposed
into an ordered plan of steps ("tracer items"). Historically that decomposition
lived in a throwaway `plan.md` document; the intent now is for it to live
**directly as tracked tickets in Sutra**, so the tickets are the single source of
truth and can be worked through by an agent.

The baseline for this feature is `develop` (slices 1–11), verified against the
code in this worktree's `internal/domain`: issues carry `parent_id` (hierarchy),
blocking edges (`blocked_by`/`is_blocking`), a nullable `project_id` (slice 10),
and an append-only ledger entry on every mutation. Issue types are `feature | bug
| task | chore`; status is `open | in_progress | closed`; `LedgerEntry.kind` is a
closed enum (`created|updated|commented|status_changed|linked|deleted`). There is
no notion of a plan, no approval checkpoint, and no way to enumerate an issue's
children. This feature is split across two slices: **@slice13** (the `plan` issue
type, its `approval` field, and atomic tree build + approve) and **@slice14** (the
`parent_id` list filter that makes the tree traversable).

A plan issue stands as the **root of its tracer tree** — the item an agent is
pointed at. It may optionally reference the feature issue it plans (as its
`parent_id`) when one exists, but does not require one; the tracer items are its
children. Note the two uses of the word "plan" are distinct axes: the new issue
**type** `plan` (this feature) is unrelated to the existing document **kind**
`plan` in `problem|design|plan|scenarios`.

## Problem

An agent cannot turn a feature's plan into a tracked, approvable, executable
ticket tree in one reliable action. Concretely, all of the following are missing
today:

- There is no first-class work item that **represents a plan as a whole** —
  carrying its overall prose (overview, rationale) and standing as the parent of
  its steps.
- There is no **approval checkpoint**: nothing lets a human sign off on a proposed
  plan before its tracer items are treated as live work.
- Creating the parent plus one ticket per tracer item plus the run-order links
  between them is **manual and non-atomic** — a partial result leaves a
  half-built tree.
- The tracer items under a plan **cannot be enumerated** (`list` filters on
  status/type/priority/owner/label but not on parent), so an agent pointed at the
  plan cannot work through it in order or mark items complete as it goes.

The tracer-item structure is authored by an agent as free-form prose; Sutra has
no way to derive structured steps from that prose.

## Complexity drivers

- **Scale**: N/A — a plan has a bounded, small number of tracer items (single
  digits to low tens). No non-linear growth.
- **Concurrency**: N/A for contention — single-user, single-writer, writes
  serialized on one connection. But building a plan creates **many linked entities
  at once** (the plan issue + per-item issues + ordering edges + ledger rows); a
  partial result would leave an inconsistent, half-built tree.
- **Failure modes** — must be prevented loudly:
  - **Malformed input** (an item with no subject, an invalid type/priority enum)
    would create invalid or unusable work items.
  - A **partial build** (source of one of the linked entities fails mid-way) must
    not leave a half-linked tree.
  - **Approving a non-plan issue**, or approving nothing, must be rejected rather
    than silently mutating an unrelated issue.
- **Cross-cutting policies**: every issue-changing operation must write a ledger
  entry in the same transaction and bump `updated_at` (existing invariant). No
  PII/secrets involved.

## Constraints

- Go 1.26, single static binary; module import allowlist enforced by
  `architecture_test.go` — no new cross-module import.
- CLI and TUI are thin clients over the daemon's HTTP API; behavior lives in
  `service`/`store`.
- Sutra must not parse markdown to derive tracer items — the structured items are
  supplied by the caller (the agent).
- The approval action must be usable by the human from the TUI (the primary human
  workspace); the full flow (build + approve) must be usable by the agent from the
  CLI (the agent workspace).
- BDD-first: godog scenarios drive the work; the roborev gate must pass before the
  slice is done.

## Non-goals

- **Storing, approving, or expanding a separate plan _document_.** The plan is now
  a first-class issue type, not a `Document{Kind:"plan"}`. The earlier
  approve/expand-a-plan-doc design is superseded by this one. (Other DDD docs —
  problem/design/scenarios — may
  still attach to the plan issue as documents; that is unchanged and out of scope
  here.)
- Non-linear dependency graphs between items (only a straight sequential chain).
- Human authoring of tracer items in the TUI — the TUI covers approval and viewing
  the result; item authoring is the agent's job via the CLI.
- Editing or re-syncing the tracked work after the fact when the plan changes.
- Un-approving a plan: approval is one-way (a plan stays approved once approved).
- Automatic status rollup on the plan: closing the last tracer does **not**
  auto-close the plan issue — the agent/human closes it explicitly.
- Distinguishing *why* an item closed (complete vs cancelled vs superseded): Sutra
  has no `resolution` concept today; `closed` means "done" for this feature.
  Resolutions are a separate follow-up (see Open questions).

## Success criteria

- A new issue type **`plan`** exists. A plan issue's body holds the plan's overall
  prose, and it carries an **approval status** that starts `pending` and moves
  one-way to `approved`.
- A single action, given the plan prose and an ordered list of tracer items,
  atomically produces a tracked tree — the plan issue plus one tracked item per
  tracer item — that an agent can work in the plan's intended order. If the action
  fails partway, nothing is created.
- Approving a plan issue flips `pending → approved`; the change is durable
  (survives restart) and observable in the issue's history/ledger. Approving a
  non-plan issue (or an absent one) is refused.
- Malformed tracer input is refused with nothing created.
- The tree is **traversable**: an agent pointed at the plan issue can, in a single
  first-class query, list its tracer items — in run order and with each item's
  current status — and mark items complete as it finishes them.
- Approval is reachable by the human without leaving the TUI; the full
  build-and-approve flow is reachable by the agent without leaving the CLI.
- The generated plan issue and its tracer children are **project-scoped
  consistently**: when the plan references a feature issue (or a project is
  supplied), the whole tree carries that `project_id`; otherwise it is unset.

## Open questions

- [ ] **Approval vocabulary.** `pending` / `approved` is the working choice;
  confirm against any existing status vocabulary in the docs during design.
- [ ] **How approval coexists with `status`.** Is approval a **new field** on the
  issue (distinct from the `open|in_progress|closed` status), a **new status enum
  value**, or something else? A design-level modeling decision.
- [ ] **How approval is recorded in the ledger.** `LedgerEntry.kind` is a closed
  enum with no approval kind. Does approval extend the enum (e.g. `approved`), ride
  on `status_changed`, or use `updated`? Resolve in design.
- [ ] **Does approval _enforce_ anything, or is it advisory?** Leaning advisory —
  Sutra is a passive tracker and should not block child mutations before approval;
  approval is a recorded human signal the agent is expected to honor. Confirm in
  design.

## Change log

- 2026-07-20: Initial draft (Brent Hoover)
- 2026-07-20: Pivoted from "approve/expand a plan document" to "plan is a
  first-class issue type built directly as tickets, gated by an approval status on
  the plan issue" (Brent Hoover)
