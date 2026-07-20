---
title: Plan Tickets — Scope
type: scope
status: draft
owner: Brent Hoover
created: 2026-07-20
design: ./design.md
---

# Plan Tickets — Scope

**Objective (one sentence):** Let an agent build a feature's plan directly as a
tracked ticket tree — a first-class `plan` issue (carrying a `pending→approved`
approval gate) plus one ordered, sequentially-blocked tracer child per step — and
let that tree be enumerated and worked to completion.

Delivered as two slices:

- **@slice13 — Plan tickets:** the `plan` issue type, the `approval` field, atomic
  tree build, and approve.
- **@slice14 — Plan traversal:** a `parent_id` filter on `list` so the tree can be
  enumerated in order with status.

## In scope

* New issue type `plan` and a new `approval` field (`pending|approved`) on `Issue`.
* `BuildPlan` — one transaction creating the plan issue + tracer children + the
  sequential `issue_block` chain + `created` ledger entries; optional parent
  (feature issue) and project inheritance.
* `ApprovePlan` — one-way `pending→approved` with an `updated` ledger entry;
  idempotent; rejects non-plan / missing targets.
* `parent_id` filter on `ListIssues` (domain filter → store → api query param →
  client → `sutra list --parent`).
* CLI `plan build` / `plan approve` commands; `list --parent` flag.
* TUI: show a plan issue's `approval` on the detail view and an approve keypress.
* BDD scenarios wired into `features/` and `implementedTags` (`@slice13`,
  `@slice14`), with step definitions.
* Schema doc update in `docs/data-models.md`.

## Out of scope / non-goals

* Any plan **document** handling (parse/approve/expand a `Document{Kind:"plan"}`) —
  the superseded design at `docs/superpowers/specs/2026-07-20-plan-expansion-design.md`.
* Non-linear dependency graphs; only a sequential chain.
* TUI authoring of tracer items or the `plan build` flow (CLI/agent only).
* Un-approving; auto-closing the plan when tracers close; `resolution` (why an
  item closed).
* Editing/re-syncing the tree after the plan changes.

## Files/areas you MAY modify (allowlist)

**Shared / domain**
* `internal/domain/issue.go` — add `TypePlan`; `Approval` type + `Issue.Approval`
  field + `Valid()`; add `ParentID` to `IssueFilter`.

**@slice13 — build + approve**
* `internal/store/plan.go` *(new)* — `BuildPlan`, `ApprovePlan`.
* `internal/store/issues.go` — extend `issueColumns`/`scanIssue`/`CreateIssue`
  insert for the `approval` column; add the `migrateApproval` method.
* `internal/store/store.go` — register the `migrateApproval` call in `migrate()`
  (migrations are not auto-discovered).
* `internal/service/plan.go` *(new)* — `PlanStep`, `BuildPlan`, `ApprovePlan`.
* `internal/api/plan.go` *(new)* + `internal/api/api.go` — `POST /plans`,
  `POST /issues/{id}/plan/approve`, route registration.
* `internal/client/plan.go` *(new)* — `BuildPlan`, `ApprovePlan`.
* `internal/cli/plan.go` *(new)* + `internal/cli/root.go` — `plan build`,
  `plan approve`.
* `internal/tui/detail.go` (+ `internal/tui/model.go`, `internal/tui/commands.go`
  as needed) — approval display + approve keypress.
* `bdd_test.go` — add `@slice13` to `implementedTags`.
* `bdd_slice13_test.go` *(new)* — step definitions.
* `features/plan-build.feature`, `features/plan-approval.feature` *(new)*.

**@slice14 — traversal**
* `internal/store/issues.go` — apply `f.ParentID` in `ListIssues`.
* `internal/api/issues.go` — parse `?parent=` into the filter.
* `internal/client/issues.go` — pass the `parent` filter through.
* `internal/cli/issue.go` — `--parent` flag on `list`.
* `internal/tui/detail.go` / `internal/tui/list.go` — show a plan's children
  (optional traversal view).
* `bdd_test.go` — add `@slice14`; `bdd_slice14_test.go` *(new)*;
  `features/plan-traversal.feature` *(new)*.

**Docs**
* `docs/data-models.md` — document the `plan` type and `approval` field.
* `docs/PLAN.md` — add the @slice13/@slice14 entries.
* `feature-work/plan-expansion/**` — these workflow docs.

## Files/areas you must NOT touch

* `internal/domain/document.go`, `internal/store/documents.go`,
  `internal/service/documents.go` — plan-as-document is dropped; no changes.
* `docs/arch-rules.yaml` and the module import graph — this feature introduces no
  new cross-module import.
* Any other slice's code (transcripts, search, activity, projects, threads, auth,
  config) except the shared `issue.go`/`issues.go`/`list` touchpoints listed above.
