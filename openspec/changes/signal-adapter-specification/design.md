## Context

See proposal.md — Why. This change builds on a sibling change,
`runtime-specification`, now archived (#373). It provides:

- The signal adapter contract already lives at
  `docs/contracts/signal-adapter.md`, moved verbatim. That sibling created the
  page. This change adds its version, changelog, routing row, publication and
  governance.
- The governance pattern (scoped rule, verify-against-spec, approval-gated
  edits, conformance carried by a spec change) is established.
- **Not provided: a generator.** `runtime-specification` added a
  `struct-table` marker to `docs-generate.py` and then removed it in the same
  change (its own commit history: "Remove the struct-table generator this
  depended on" — the Go doc comments it copied verbatim carried rationale
  normal for Go source but wrong for a spec table). `runtime.md`'s field
  tables are hand-written, requirement-ID style, same shape
  `cr-reference.md`'s generator already uses for CRD fields. This change
  follows that actual, final shape — not the generator the original proposal
  assumed would still exist.

This change was blocked on `runtime-specification` being archived. That
precondition is now satisfied.

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

Versioning table (MAJOR/MINOR/PATCH), the two-party split, requirement-ID
prefixes, anchors referenced by name, hand-written field tables, publication
(front matter + nav entry), governance as a scoped rule — all exactly as
`runtime.md` ships them today. This design states only what differs for
signal adapters.

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

- [Five shipped adapters drift from the spec] → verify each adapter's own
  tests against the spec by hand before claiming its version, and fix any
  gap in this change. The page itself states a minimal/optional split, not a
  per-adapter test registry — matching `runtime.md`'s own Conformance
  section, which dropped the test-citing form as unreadable.
- [The self-exclusion invariant is restated wrongly] → the spec cites
  `signals/ha/selfexclude.go`, `signals/k8s-events/selfexclude.go` and the invariant, never paraphrases the rule's mechanics.
- [Governance rule is prose, not enforcement] → accepted, same trade as the
  sibling changes.

## Migration Plan

Documentation only. No deploy step, rollback is reverting the PR.
