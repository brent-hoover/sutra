---
title: Plan Tickets — Implementation Plan
type: plan
status: draft
owner: Brent Hoover
created: 2026-07-20
updated: 2026-07-20
design: ./design.md
---

# Plan Tickets — Implementation Plan

## Overview

Two slices, sequenced back-to-back, built bottom-up. **@slice13** adds the `plan`
issue type, the `approval` field, the atomic `BuildPlan`, and `ApprovePlan`
(domain → store → service → api/client → cli), *then* wires the godog scenarios
green, then the TUI face. **@slice14** adds the `parent_id` list filter that makes
the tree traversable (domain → store → api/client → cli), then wires its scenarios
green. **The godog harness drives every scenario through the real `sutra` CLI
against a live daemon (full stack `cli→client→http→api→service→store`), so a
scenario can only go green after its API *and* CLI paths exist** — hence the
scenario-wiring step comes last in each slice, not first. The `.feature` files are
authored up front (already drafted); each slice is done only when its scenarios
pass and roborev returns P.

## Preconditions

- [x] `design.md` approved
- [x] All `.feature` files in `scenarios/` approved
- [x] `scope.md` filled with objective, allowlist, and non-goals
- [x] Open questions resolved (approval modeling/vocabulary/ledger)
- [x] Worktree rebased onto `develop` (slices 1–11; `project_id` present)

---

## Steps — @slice13 (Plan tickets)

### 1. Domain: `plan` type + `approval` field

**What:** `internal/domain/issue.go` — add `TypePlan IssueType = "plan"` (include
in `IssueType.Valid()`); add `type Approval string` with `ApprovalPending`,
`ApprovalApproved`, and `Approval.Valid()`; add `Approval Approval` to `Issue`
(`json:"approval,omitempty"`).

**Why:** foundation every other layer depends on.

**Scenarios:** none pass yet (pure types).

**Verify:** `go build ./...` && `go vet ./...` — exit 0.

### 2. Store: `approval` column round-trip

**What:** add a `migrateApproval` method
(`ALTER TABLE issues ADD COLUMN approval TEXT NOT NULL DEFAULT ''`) and **register
its call in `migrate()` in `internal/store/store.go`** (migrations are not
auto-discovered). Extend `issueColumns`, `scanIssue`, and `CreateIssue`'s insert in
`internal/store/issues.go` so `approval` reads/creates correctly. Leave the generic
`UpdateIssueTx` `SET` clause untouched (that omission preserves `approval` across
ordinary updates).

**Why:** persist the new field before anything writes it.

**Verify:** `go test ./internal/store/` — a round-trip test creates an issue and
reads `approval` back as `""`. Exit 0.

### 3. Store: `BuildPlan` + `ApprovePlan`

**What:** new `internal/store/plan.go`: `BuildPlan` (one tx — resolve parent
existence + project inheritance in-tx, insert plan then children with
strictly-increasing `created_at`, `issue_block` chain, `created` ledger per issue)
and `ApprovePlan` (one tx — `ErrNotFound`/non-plan `ErrInvalidIssue`, idempotent
when already approved, else set `approval`, bump `updated_at`, `updated`/`approval`
ledger).

**Why:** atomic persistence and the approval transition.

**Verify:** `go test ./internal/store/` — new `plan_test.go` covers build atomicity
(rollback on bad child leaves nothing), monotonic order, project inheritance,
approve idempotency, and non-plan rejection. Exit 0.

### 4. Service: `PlanStep`, `BuildPlan`, `ApprovePlan`

**What:** new `internal/service/plan.go` — `PlanStep{Subject,Body,Type,Priority}`;
`BuildPlan(title, prose, steps, parentID, projectID)` generating all ids up front
(so `child.ParentID = plan.ID`), validating (non-empty title/prose/steps, non-empty
subjects, valid enums), resolving defaults (`task`/`p2`, empty body → subject),
calling `store.BuildPlan`; `ApprovePlan(id)` stamping the ledger entry and calling
`store.ApprovePlan`.

**Why:** the validation boundary and use-case orchestration.

**Verify:** `go test ./internal/service/` — new `plan_test.go` covers validation
rejections and the happy path. Exit 0.

### 5. API + client: build + approve endpoints

**What:** new `internal/api/plan.go` — `POST /plans` and
`POST /issues/{id}/plan/approve` (400 invalid enum/empty; 404 missing
issue/parent/project; 400 non-plan approve); register in `internal/api/api.go`.
New `internal/client/plan.go` — `BuildPlan`, `ApprovePlan`.

**Why:** expose the behavior over HTTP — required before the CLI or any godog
scenario can reach it.

**Verify:** `go test ./internal/api/ ./internal/client/` — new tests assert status
codes and round-trip. Exit 0.

### 6. CLI: `plan build` / `plan approve`

**What:** new `internal/cli/plan.go` (registered in `internal/cli/root.go`):
`plan build --title --prose <file|-> --steps <file|-> [--parent] [--project]`
(reads prose + JSON steps from file/stdin; prints plan id, child ids, chain,
sanitized via `safe.go`); `plan approve <plan-id>`.

**Why:** the agent's entry point — and the surface the godog harness drives.

**Verify:** `go test ./internal/cli/` (flag parsing/steps decode) + manual:
`sutra plan build …` then `sutra plan approve <id>`; observe the tree. (Full BDD
proof lands in Step 7.)

### 7. Wire @slice13 scenarios to green

**What:** copy `scenarios/plan-build.feature` and `scenarios/plan-approval.feature`
into `features/`; add step definitions in new `bdd_slice13_test.go` (driving the
`sutra` CLI via the existing `runCLI` harness); append `@slice13` to
`implementedTags` in `bdd_test.go` — **comma-joined** (`"…,@slice11,@slice13"`),
never `||`.

**Why:** the executable definition of done for build + approve, exercised full-stack.

**Scenarios:** `features/plan-build.feature` — "Build a plan into a ticket tree",
"Building a plan rejects invalid tracer input"; `features/plan-approval.feature` —
"Approve a pending plan", "Approval rejects invalid targets".

**Verify:** `go test -count=1 -run TestImplemented ./...` — the 4 new @slice13
scenarios pass; total scenario count rises by 4.

### 8. TUI: approval display + approve keypress

**What:** `internal/tui/detail.go` — render `approval: pending|approved` for
`plan`-type issues; add an approve keypress in `handleDetailKey` calling
`client.ApprovePlan` and reloading (no-op message when not a pending plan); update
the footer hint. Touch `internal/tui/model.go`/`commands.go` only as needed for the
command wiring.

**Why:** the human's approval workspace.

**Verify:** `go test ./internal/tui/` + manual: open a pending plan in the TUI,
press the approve key, see it flip to `approved`.

---

## Steps — @slice14 (Plan traversal)

### 9. Domain + store: `parent_id` filter

**What:** add `ParentID string` to `domain.IssueFilter`; apply it in
`store.ListIssues` (`AND parent_id = ?`), mirroring the existing `ProjectID`
clause. Children already return in `created_at, id` order (Step 3 made that tracer
order).

**Why:** enumerate a plan's children.

**Verify:** `go test ./internal/store/` — list-by-parent returns only children, in
order. Exit 0.

### 10. API + client + CLI: expose the filter

**What:** `internal/api/issues.go` — read `?parent=` into the filter (no enum
validation; the existing status-enum rejection there is what the "invalid filter"
scenario relies on). `internal/client/issues.go` — pass the `parent` key through
`ListIssues`. `internal/cli/issue.go` — `--parent <id>` flag on `list`.

**Why:** make traversal reachable from the CLI/daemon — required before the
traversal scenarios (which drive `sutra list --parent`) can go green, and the only
layer where the invalid-status-filter rejection exists.

**Verify:** `go test ./internal/api/ ./internal/client/` + manual
`sutra list --parent <plan-id>`. (Full BDD proof lands in Step 11.)

### 11. Wire @slice14 scenarios to green

**What:** copy `scenarios/plan-traversal.feature` into `features/`; add step
definitions in new `bdd_slice14_test.go` (driving `sutra list --parent` via the
harness); append `@slice14` to `implementedTags` — **comma-joined**.

**Why:** the executable definition of done for traversal.

**Scenarios:** `features/plan-traversal.feature` — "Enumerate a plan's tracers in
order", "Completing a tracer is reflected on the next traversal".

**Verify:** `go test -count=1 -run TestImplemented ./...` — the 2 new @slice14
scenarios pass; total count rises by a further 2.

### 12. TUI: show a plan's children

**What:** `internal/tui/detail.go` — when viewing a `plan` issue, list its children
(via the `parent` filter) with status, so the human can see the tree.

**Why:** the human's "view the result" path — a required part of the TUI face, not
a nice-to-have; approving a plan you can't see the shape of is half a workflow.

**Verify:** `go test ./internal/tui/` + manual: open a plan, see its tracers listed.

### 13. Docs + finalize

**What:** update `docs/data-models.md` (the `plan` type, the `approval` field) and
add the @slice13/@slice14 entries to `docs/PLAN.md`.

**Why:** keep the schema source of truth current.

**Verify:** `go build ./...` && `go vet ./...` && `go test -race ./...` (whole
suite green); confirm the final scenario count; then the roborev gate
`/roborev-refine --since <base-commit>` returns P for each slice.

---

## Scenario coverage

Each scenario goes green at its wiring step (7 or 11), which depends on all the
layers below it — the harness drives the full CLI→daemon stack, so the API and CLI
steps are load-bearing for every scenario, not just the store/service ones.

| Feature file | Scenario | Green at | Depends on |
|---|---|---|---|
| `plan-build.feature` | Build a plan into a ticket tree | 7 | 1, 3, 4, 5, 6 |
| `plan-build.feature` | Building a plan rejects invalid tracer input | 7 | 4, 5, 6 |
| `plan-approval.feature` | Approve a pending plan | 7 | 1, 2, 3, 4, 5, 6 |
| `plan-approval.feature` | Approval rejects invalid targets | 7 | 1, 3, 4, 5, 6 |
| `plan-traversal.feature` | Enumerate a plan's tracers in order | 11 | 9, 10 |
| `plan-traversal.feature` | Completing a tracer is reflected on the next traversal | 11 | 3, 9, 10 |

No scenario is left uncovered. CLI/TUI-only surfaces (Steps 8, 12) are verified by
package-level Go tests + manual smoke, per the design's "Verification of the
CLI/TUI faces" note; their behaviors are also exercised full-stack by the wiring
steps.

## Rollback

Work is confined to the `feature/plan-to-tickets` worktree/branch. If it goes
sideways: `git reset --hard` to the last green step, or abandon the branch (nothing
merges to `develop` until both slices are green and roborev-passed). The
`approval` column is additive with a default, so an un-merged migration leaves
`develop` unaffected.

## Out of scope for this plan

- Plan documents (parse/approve/expand) — superseded design.
- `resolution` on close, auto-close rollup, un-approve, non-linear graphs, re-sync.

## Change log

- 2026-07-20: Initial draft (Brent Hoover)
