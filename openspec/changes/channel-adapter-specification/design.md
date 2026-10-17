## Context

See proposal.md — Why. `runtime-specification` lands the structure this change
builds on:

- The channel adapter contract already lives at
  `docs/contracts/channel-adapter.md`, moved verbatim.
- `docs-generate.py` already has the `struct-table` marker.
- The governance pattern (scoped rule, verify-against-spec, approval-gated
  edits, conformance carried by a spec change) is established.

This change MUST NOT start before `runtime-specification` is archived.

## Goals / Non-Goals

**Goals:**

- A channel adapter built from the specification alone works first attempt.
- The spec page cannot rot silently where it is struct-backed.

**Non-Goals:**

- No behavior change to the manager, `channels/telegram`, the console or the
  chart.
- No change to the capability's behavioral requirements — this adds the
  document-governance layer only.

## Decisions

### Shared decisions are adopted, not re-made

Versioning table (MAJOR/MINOR/PATCH), two-genre split with one-directional
references, anchors referenced by name, generated tables, governance as a
scoped rule — all exactly as `runtime-specification`'s design.md decides them.
This design states only what differs for channel adapters.

### Struct-backed and grammar-backed parts are named apart

The typed operations and their payloads are Go structs and get generated
tables. The body grammar is a grammar, not a struct — it stays hand-written,
shared with `structured-agent-output`, and the spec names which sections are
which.

### The console is a shipped channel adapter

`platform/console` consumes the same contract, so it claims a conformance
version like `channels/telegram` does. Its claim goes on `docs/console.md`,
the reference page that owns what the console IS.

### The spec starts at 1.0.0 and documents current behavior

Nothing shipped changes to conform. Both shipped adapters claim 1.0.0 at
birth.

## Risks / Trade-offs

- [The ops surface is wider than the runtime's — the spec balloons] → the
  behavioral requirements already exist in the capability. The page states
  them once each, tables over prose, and the checklist is the length control.
- [Two shipped adapters drift from one checklist] → each checklist item cites
  the test that exercises it per adapter, and a gap is recorded as a gap
  rather than papered over.
- [Governance rule is prose, not enforcement] → accepted, same trade as the
  sibling change.

## Migration Plan

Documentation only. No deploy step, rollback is reverting the PR.
