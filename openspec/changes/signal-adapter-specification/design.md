## Context

See proposal.md — Why. This change builds on a sibling change,
`runtime-specification`, which is not yet in the tree (no change directory or
archive entry exists). Once it lands it provides:

- The signal adapter contract already lives at
  `docs/contracts/signal-adapter.md`, moved verbatim. That sibling creates the
  page. This change only adds its version, changelog, routing row and
  governance.
- `docs-generate.py` already has the `struct-table` marker.
- The governance pattern (scoped rule, verify-against-spec, approval-gated
  edits, conformance carried by a spec change) is established.

This change MUST NOT start before `runtime-specification` is archived.

## Goals / Non-Goals

**Goals:**

- A signal adapter built from the specification alone works first attempt.
- The inbound payload table cannot rot silently.

**Non-Goals:**

- No behavior change to the manager, any shipped signal adapter or the chart.
- No change to the capability's behavioral requirements — this adds the
  document-governance layer only.

## Decisions

### Shared decisions are adopted, not re-made

Versioning table (MAJOR/MINOR/PATCH), two-genre split with one-directional
references, anchors referenced by name, generated tables, governance as a
scoped rule — all exactly as `runtime-specification`'s design.md decides them.
This design states only what differs for signal adapters.

### This is the smallest of the three specifications

A signal adapter is inbound-only, one endpoint, no ops queue. The spec stays
one party short of the channel one in surface, and the station procedure is
correspondingly shorter. Brevity is correct here, not a gap.

### Version claims land where each adapter's page is

Four shipped adapters have integration pages and claim there. `signals/cron`
has no page of its own — its documentation home is the guide, so its claim
lives there, stated as cron's rather than the guide's.

### The spec starts at 1.0.0 and documents current behavior

Nothing shipped changes to conform. All five shipped adapters claim 1.0.0 at
birth.

## Risks / Trade-offs

- [Five shipped adapters drift from one checklist] → each checklist item
  cites the test that exercises it per adapter, and a gap is fixed in this
  change before the version is claimed.
- [The self-exclusion invariant is restated wrongly] → the spec cites
  `selfexclude.go` and the invariant, never paraphrases the rule's mechanics.
- [Governance rule is prose, not enforcement] → accepted, same trade as the
  sibling changes.

## Migration Plan

Documentation only. No deploy step, rollback is reverting the PR.
