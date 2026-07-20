---
title: Plan Tickets — Design
type: design
status: draft
owner: Brent Hoover
created: 2026-07-20
updated: 2026-07-20
problem: ./problem.md
---

# Plan Tickets — Design

## Summary

Add a first-class **`plan`** issue type. An agent builds a feature's plan directly
as a ticket tree in one call: a `plan` issue (its body holds the plan prose,
carrying an `approval` field that starts `pending`), one child issue per tracer
item, and a sequential `issue_block` chain so tracer *N* blocks *N+1* — all in one
store transaction with a `created` ledger entry per new issue. A human approves
the plan issue (`pending → approved`, one-way) as the gate. A new `parent_id`
filter on `list` makes the tree enumerable so an agent can work it to completion,
marking each tracer `closed`. Everything reuses existing mechanisms
(`issue_block`, the ledger, `IssueFilter`); no new cross-module import.

## BDD Scenarios

| Feature file | Behavior proven |
|---|---|
| `scenarios/plan-build.feature` | Building a `plan` issue + ordered tracer children + blocking chain; ledger entries; single-tracer edge; project scoping (parent-inherited, explicitly-supplied, and unset); rejection of malformed tracer input (empty subject, invalid type, empty steps). |
| `scenarios/plan-approval.feature` | One-way `pending→approved` with an `updated`/`approval` ledger entry; idempotent re-approve; rejection of non-plan and missing targets. |
| `scenarios/plan-traversal.feature` | Enumerating a plan's tracers via the `parent` filter in run order with status; empty result for a childless issue; invalid status filter rejected; a closed tracer reflected as `closed` on the next traversal (block edge retained). |

Run all scenarios: `go test -run TestImplemented ./...` (tags `@slice13,@slice14`).

**Verification of the CLI/TUI faces.** The godog scenarios exercise the
service/store behavior (as the existing slices do). The thin CLI/TUI faces —
`sutra plan build`, `sutra plan approve`, `sutra list --parent`, and the TUI
approve keypress — are verified by package-level Go tests (`internal/client`,
`internal/cli`, `internal/tui`) plus a manual smoke run, mirroring how slice 8
(TUI) was verified. Durability of `approval` ("survives restart") is inherent to
the SQLite column and needs no dedicated scenario.

## Approach

Two slices; the ordering is: type + field + build + approve (@slice13), then the
traversal filter (@slice14).

**Domain (`internal/domain/issue.go`).**
- Add `TypePlan IssueType = "plan"`; include it in `IssueType.Valid()`.
- Add `type Approval string` with `ApprovalPending = "pending"`,
  `ApprovalApproved = "approved"`, and `Approval.Valid()`. Add
  `Approval Approval` to `Issue` (`json:"approval,omitempty"`). It is empty for
  non-plan issues and set only on `plan` issues.
- Add `ParentID string` to `IssueFilter` (@slice14).

**Store.**
- Migration: `ALTER TABLE issues ADD COLUMN approval TEXT NOT NULL DEFAULT ''`.
  Extend `issueColumns`, `scanIssue`, and `CreateIssue`'s insert so `approval`
  round-trips on read/create. The generic `UpdateIssueTx` `SET` clause is left
  untouched (it omits `approval`, which is exactly what preserves the value across
  ordinary field updates); only `ApprovePlan` writes the column, in its own
  transaction.
- `internal/store/plan.go` (new):
  - `BuildPlan(plan domain.Issue, children []domain.Issue) (domain.Issue, []domain.Issue, error)`
    — one transaction: if `plan.ParentID` is set, verify it exists; resolve the
    project (if `plan.ProjectID` is nil and a parent is set, inherit the parent's
    `project_id`) and stamp it on the plan and every child; insert the plan issue,
    then each child (each already carrying `ParentID = plan.ID`), then
    `issue_block` rows `(child[i].ID, child[i+1].ID)`; append a `created`
    `LedgerEntry` for the plan and each child. Rolls back on any failure, so a
    partial tree is never visible.
    - **Run order is made deterministic here**: children are stamped with a
      strictly-increasing `created_at` (child *i* gets `base + i`), so the existing
      `ORDER BY created_at, id` in `ListIssues` returns them in tracer order
      without a random `id` tiebreak. (Considered and rejected: an explicit
      `position` column — narrower use, more schema; deriving order by walking the
      `issue_block` chain in the list path — couples `list` to blocking.)
  - `ApprovePlan(id string, entry domain.LedgerEntry) (domain.Issue, error)` — one
    transaction: load the issue (`ErrNotFound` if absent); reject if
    `type != plan` (`ErrInvalidIssue`); if already `approved`, idempotent no-op
    (no ledger, return as-is); else set `approval = approved`, bump `updated_at`,
    append the `updated`/`approval` ledger entry.
- `internal/store/issues.go` (@slice14): apply `f.ParentID` in `ListIssues`
  (`AND parent_id = ?`), mirroring the existing `ProjectID` clause. Children come
  back in `created_at, id` order — i.e. tracer order — which is the run order.

**Service (`internal/service/plan.go`, new).**
- `type PlanStep struct { Subject, Body string; Type domain.IssueType; Priority domain.Priority }`.
- `BuildPlan(title, prose string, steps []PlanStep, parentID, projectID *string) (domain.Issue, []domain.Issue, error)`
  — validate: `title` and `prose` non-empty; `steps` non-empty; each step's
  `Subject` non-empty; any non-empty `Type`/`Priority` valid via `.Valid()`.
  Resolve defaults (step `Type→task`, `Priority→p2`; empty step `Body→Subject` so
  the non-empty-body invariant holds). Compose the plan `domain.Issue`
  (`Type=plan`, `Approval=pending`, `Body=prose`, `Subject=title`, `ParentID`,
  `ProjectID`) and the child issues (`ParentID = plan.ID` is set by the store once
  the plan id exists — service passes children with a sentinel and the store wires
  parent, or service generates the plan id up front; **decision:** service
  generates all ids via `domain.NewID()` so it can set `child.ParentID = plan.ID`
  before calling the store). Call `store.BuildPlan`.
- `ApprovePlan(id string) (domain.Issue, error)` — stamp a ledger entry
  (`kind=updated, field="approval", new_value="approved"`) and call
  `store.ApprovePlan`.

**API.**
- `internal/api/plan.go` (new): `POST /plans` with body
  `{ "title", "prose", "parent_id"?, "project_id"?, "steps":[{subject,body,type?,priority?}] }`
  → `BuildPlan`; `POST /issues/{id}/plan/approve` → `ApprovePlan`. Enum/empty
  validation → 400; missing issue/parent/project → 404; non-plan approve → 400.
  Register both in `internal/api/api.go`.
- `internal/api/issues.go` (@slice14): read `q.Get("parent")` into
  `IssueFilter.ParentID` (no enum validation — it is an id).

**Client (`internal/client/plan.go`, new).** `BuildPlan(req) (result, error)` and
`ApprovePlan(id) (domain.Issue, error)`, JSON over HTTP in the existing style.
`internal/client/issues.go` passes a `parent` filter key through `ListIssues`.

**CLI.**
- `internal/cli/plan.go` (new), registered in `root.go`:
  - `sutra plan build --title <t> --prose <file|-> --steps <file|-> [--parent <id>] [--project <id>]`
    — reads prose and the JSON steps array from a file or stdin; prints the plan
    id, child ids, and the chain. Output sanitized via `safe.go`.
  - `sutra plan approve <plan-id>` — prints the approved plan summary.
- `internal/cli/issue.go` (@slice14): add `--parent <id>` to `list`.

**TUI (`internal/tui/detail.go`).** On the detail view, render `approval:
pending|approved` for `plan`-type issues. Add an approve keypress (e.g. `A`) in
`handleDetailKey` that calls `client.ApprovePlan` and reloads; a no-op with a
status message when the issue is not a pending plan. In @slice14 the detail view
also lists a `plan` issue's children (via the `parent` filter) with status, so the
human can see the tree they are approving. `build` is **not** surfaced in the TUI
(agent/CLI only).

## Interfaces

- **HTTP:** `POST /plans`, `POST /issues/{id}/plan/approve`, and the new
  `?parent=<id>` query param on `GET /issues`.
- **CLI:** `sutra plan build …`, `sutra plan approve <id>`, `sutra list --parent <id>`.
- **Steps JSON (agent-authored):**
  `[{"subject":"…","body":"…","type":"task","priority":"p2"}, …]` — `type`/`body`/
  `priority` optional per item.

## Data model

- `issues.approval TEXT NOT NULL DEFAULT ''` — `''` for non-plan issues,
  `pending|approved` for plan issues.
- No new tables. Ordering reuses `issue_block`; grouping reuses `parent_id`;
  the epic↔feature link reuses `parent_id` (plan issue's own parent).
- `LedgerEntry.kind` is unchanged (closed enum); approval rides on `updated` with
  `field="approval"`.

## Alternatives considered

### Simplest

Agent creates the plan issue and children with the existing `CreateIssueWith` +
`AddBlock` calls, no new server code; approval is just a label. **Drawbacks:**
non-atomic (a failure leaves a half-built tree), no first-class `plan` type or
approval semantics, and the ordering/approval conventions live only in the agent —
exactly the drift the problem statement rejects.

### Complete (chosen)

A `plan` issue type + `approval` field + a single atomic `BuildPlan` store call +
`ApprovePlan` + a `parent_id` list filter. Reuses `issue_block`, the ledger, and
`IssueFilter`; adds no table and no cross-module import. Handles the complexity
drivers (atomicity, malformed-input rejection, approval gate) without new
mechanism beyond one column.

### Optimal

Model the plan/tracer lifecycle richly: a `resolution` on close
(complete/cancelled/superseded), progress rollup on the plan, and re-sync when the
plan changes. **Trades away:** scope and time; these are separate cross-cutting
features (resolutions already logged as a follow-up). Not needed for the agent
loop now.

**Decision:** Complete. Scale/concurrency don't push past it; the failure-mode and
approval-gate drivers push above Simplest; the resolution/rollup ambitions are
deferred, keeping us below Optimal.

## Risks

- **`approval` as a second lifecycle axis** alongside `status` could confuse
  readers. Mitigation: it is empty for non-plan issues and only ever `pending`/
  `approved`; documented in `data-models.md`.
- **Project inheritance timing** — resolving the parent's `project_id` must happen
  inside the `BuildPlan` transaction to avoid a TOCTOU where the parent's project
  changes mid-build. Designed to read the parent within the tx.
- **Slice split** — @slice14 (traversal) is what makes the tree usable; shipping
  @slice13 alone yields a tree that's hard to enumerate. Both must land for the
  success criteria to hold; the plan sequences them back-to-back.

## Out of scope

- Plan **documents** (parse/approve/expand) — superseded.
- Resolutions, auto-close rollup, un-approve, non-linear graphs, plan re-sync.

## Open questions

- [ ] None blocking. (`approval` vocabulary, approval-vs-status modeling, and
  ledger recording were resolved during design: `pending`/`approved`, a distinct
  `approval` field, recorded via `updated`.)

## Change log

- 2026-07-20: Initial draft (Brent Hoover)
