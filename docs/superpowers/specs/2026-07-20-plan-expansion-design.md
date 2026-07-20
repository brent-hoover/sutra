# Plan → Ticket-Tree Expansion — Design

**Date:** 2026-07-20
**Status:** approved (design)
**Branch:** `feature/plan-to-tickets`

## Problem

The doc-driven-development workflow produces a `plan.md` for a feature and
attaches it to an issue as a `Document{Kind: "plan"}`. Turning that plan into
trackable work is manual: someone re-types each step as an issue and wires up the
ordering by hand. We want, once a plan is approved, to generate the ticket tree
automatically: a parent epic and one child per tracer item, chained in the order
the plan dictates.

## Non-goals

- Sutra does **not** parse `plan.md` markdown. The agent that wrote the plan
  supplies the ordered tracer items as structured data. Sutra never inspects the
  document body to derive steps.
- No non-linear dependency graphs. Ordering is a straight sequential chain.
- No human step-authoring in the TUI (see TUI section).

## Shape

Two operations on an issue that already carries a `plan` document, exposed as a
`sutra plan` command group and mirrored through client → api → service → store:

```
sutra plan approve <issue-id>
sutra plan expand  <issue-id> --epic-title "…" [--epic-type feature] --steps <file|->
```

`--steps` is a JSON array the agent produces from the approved plan:

```json
[
  {"subject": "Add FTS5 schema",     "body": "…", "type": "task", "priority": "p2"},
  {"subject": "Wire search service", "body": "…"}
]
```

`type` and `priority` are optional per item; defaults `task` / `p2`. Items are
consumed in array order — index *i* is chained to block index *i+1*.

### What `expand` builds (one transaction)

1. A new **epic** issue: `type` = `--epic-type` (default `feature`), `subject` =
   `--epic-title`, body = optional `--epic-body` (Sutra does not read the plan, so
   the body is agent-supplied); if omitted it defaults to a generated note
   referencing the source issue (`"Generated from the approved plan of <id>."`),
   which also satisfies the non-empty-body invariant. `parent_id` = **the source
   issue** (source › epic › children).
2. One **child** issue per tracer item, `parent_id` = the epic.
3. A **sequential blocking chain**: child *i* blocks child *i+1* (`issue_block`
   rows).
4. Ledger entries: `created` for the epic and every child; `updated`
   ("expanded plan") on the source issue.
5. The plan document is marked **expanded** (`expanded_at`), so a second
   `expand` on the same plan is refused.

`expand` refuses unless the source issue's plan document is `approved` and not
yet `expanded`.

## Layer-by-layer

### domain (`internal/domain`)

- `Document` gains `Approved bool`, `ApprovedAt *time.Time`, `ExpandedAt *time.Time`.
- `PlanStep{ Subject, Body string; Type IssueType; Priority Priority }` — pure
  input type. Empty `Type`/`Priority` mean "use default".
- No new imports (domain still imports nothing).

### store (`internal/store`)

- Migration on `documents`: add `approved INTEGER NOT NULL DEFAULT 0`,
  `approved_at TEXT`, `expanded_at TEXT`. Extend `documentColumns` + `scanDocument`
  + `DocumentsForIssue` to read them.
- `ApprovePlanDocument(issueID string, entry domain.LedgerEntry) (domain.Document, error)`
  — finds the issue's plan doc; sets `approved`, `approved_at`; bumps issue
  `updated_at`; appends ledger. Idempotent no-op if already approved.
  `ErrNotFound` if the issue has no plan doc.
- `ExpandPlan(...)` — single transaction implementing the five steps above.
  Requires the plan doc `approved && expanded_at IS NULL`, else
  `ErrInvalidIssue` ("plan not approved" / "plan already expanded"). New issues
  are inserted directly (no cycle risk: epic's parent is a pre-existing issue,
  children's parent is the just-created epic).

### service (`internal/service`)

- `ApprovePlan(issueID) (domain.Document, error)` → `store.ApprovePlanDocument`
  with a stamped ledger entry.
- `ExpandPlan(issueID string, in ExpandInput) (epic domain.IssueView, children []domain.IssueView, err error)`
  — validates non-empty `EpicTitle`, non-empty `Steps`, rejects a step with an
  empty subject, validates any provided `Type`/`Priority`/`epic_type` via
  `.Valid()`, resolves defaults (a step's empty body defaults to its subject so
  the non-empty-body invariant holds), calls `store.ExpandPlan`, hydrates the
  views back.

### api (`internal/api`)

- `POST /issues/{id}/plan/approve` → `approvePlan`.
- `POST /issues/{id}/plan/expand`, body
  `{ "epic_title", "epic_type", "epic_body", "steps": [PlanStep…] }` → `expandPlan`.
- Status mapping: invalid enum / empty title / empty steps → 400; issue or plan
  doc missing → 404; not-approved / already-expanded → 400.

### client (`internal/client`)

- `ApprovePlan(id string) (domain.Document, error)`
- `ExpandPlan(id string, req ExpandRequest) (ExpandResult, error)`
  JSON over HTTP, matching existing client method style.

### cli (`internal/cli`)

- New `plan.go` command group registered under root:
  - `plan approve <id>` — prints the approved doc summary.
  - `plan expand <id> --epic-title [--epic-type] [--epic-body] --steps <file|->` — reads the JSON
    steps array from a file or stdin (`-`), prints the epic id, child ids, and the
    chain. All daemon strings sanitized via `safe.go` (`clean`/`cleanLine`).
- `--json` passthrough honored where present (verbatim daemon bytes).

### tui (`internal/tui`)

- Detail view (`detail.go`): in the Documents list, annotate a plan doc with its
  state — `[plan] Title (approved)` / `(expanded)` / `(draft)`.
- New keypress in `handleDetailKey` (e.g. `A`) → approve the issue's plan document
  via `client.ApprovePlan`, then reload the detail. No-op with a status message if
  the issue has no plan doc. Footer hint updated.
- **`expand` is not surfaced in the TUI** — it requires structured steps authored
  by the agent. The TUI covers approval and viewing the generated tree (the epic
  and children already render through existing parent/child + blocking display).

## Architecture impact

Everything stays within the existing import allowlist
(`domain`→∅, `store`→domain/config, `service`→domain/store/config,
`api`→domain/service/config, `client`→domain/config, `tui`→domain/client,
`cli`→domain/client/api/config). **No `docs/arch-rules.yaml` change.**

## Testing (BDD-first)

New `features/plan-expansion.feature` with a fresh tag added to
`bdd_test.go`'s `implementedTags` (comma-joined). Scenarios:

- Approve a plan document → doc becomes approved; ledger entry on the issue.
- Approve when the issue has no plan document → error.
- Approve twice → idempotent (still approved, no duplicate ledger churn).
- Expand before approval → refused.
- Expand after approval → new epic under the source issue, N children under the
  epic, sequential blocking chain (child *i* blocks *i+1*), ledger `created`
  entries, source issue `updated`.
- Expand a second time → refused (already expanded).
- Expand with empty steps / an empty subject / an invalid enum → rejected.

Verify with `go build ./...`, `go vet ./...`, `go test -race ./...`, confirm the
scenario count, then the roborev gate (`/roborev-refine --since <base>`).

## Open questions

None outstanding — all resolved during brainstorming.
