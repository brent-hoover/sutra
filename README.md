# Sutra

An issue tracker for agent-assisted development: feature issues carry their
doc-driven-development documents and link the Claude transcripts that
produced them, all searchable in one place. Single user, self-hosted.

Delivered as one Go binary — `sutra serve` (daemon), `sutra` (TUI),
`sutra <verb>` (CLI).

## Getting started

Implementation follows the plan slice by slice. **Start with
[`docs/PLAN.md`](docs/PLAN.md) slice 1** (the walking skeleton).

Each slice is BDD-first: make its godog scenarios (in `features/`) go from
red to green, then confirm the slice's Verify line.

```bash
go test -count=1 -run TestArchitecture ./...   # architecture rules — green
go test -count=1 -run TestFeatures ./...        # BDD specs — red until implemented
```

## Docs

Planning docs live in [`docs/`](docs/): vision, stack, architecture (+
`arch-rules.yaml`), data-models, STORIES, PLAN.
