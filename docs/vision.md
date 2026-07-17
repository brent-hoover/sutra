---
phase: vision
status: approved
approved: 2026-07-16
owner: Brent Hoover
date: 2026-07-16
---
<!-- CONTRACT: States what the product is, for whom, and what success looks
     like. NO technology choices — those live in stack.md. Module boundaries
     live in architecture.md. This doc is the sole home of out-of-scope. -->

# Sutra — Vision

## What & who
Sutra is an issue tracker for agent-assisted development. Beyond ordinary
issues, a feature issue carries the full set of doc-driven-development
documents that describe it, and it links the Claude transcripts that
produced the work — all searchable in one place. It has a single user: the
developer running it for their own projects.

## Problem
The developer already tracks work with an issue tracker and follows a
BDD/doc-driven-development flow that generates documents (problem, design,
plan, and so on) plus long Claude transcripts. Today those threads and docs
live scattered across separate places, so it is hard to find which
transcript produced a given doc, or gather everything tied to one feature.
Keeping issues, their docs, and their transcripts together removes that
loss-of-thread and makes past work findable.

## System shape
- **Delivery form:** daemon, TUI, and CLI over a shared local data store.
- **Topology:** single self-hosted service — the daemon runs on one machine
  on the developer's home network and serves the TUI and CLI from all their
  computers.
- **Major components:**
  - Issue store — issues with types; a feature-type issue holds its
    doc-driven-development documents.
  - Transcript capture — ingests Claude transcripts and links them to issues.
  - Search — queries across issues, documents, and transcripts.
  - Daemon — the shared service the TUI and CLI connect to.
  - TUI and CLI — the two interactive front ends.

## Success criteria
1. Create an issue and set its type; a feature-type issue can hold the
   doc-driven-development documents that describe it.
2. Attach and read those documents on a feature issue.
3. Ingest a Claude transcript, link it to an issue, and later find the
   transcript that produced a given document or issue.
4. Full-text search a known term and have every issue, document, and
   transcript containing that term appear in the results.
5. Run the daemon on one home-network machine and use the TUI and CLI
   against it from any of the developer's computers.

## Out of scope (v1)
- Cloud/off-site hosting — self-hosted on the home network only.
- Multiple users, teams, or multi-tenant access — single user only.
- Transcripts from agents other than Claude.
