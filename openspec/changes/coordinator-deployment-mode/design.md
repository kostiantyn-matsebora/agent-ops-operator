## Context

`coordinated-agents` (ADR 0002) shipped the `Coordinator` and
`AgentCapability` CRDs, the `invoke`/`close`/`escalate`/`read` verbs on
`platform/mcp-aops`, per-Coordinator budgets, and result routing from a
member to its parent.

See `proposal.md` — Why for the gap this change closes: nothing wires a
Coordinator by default, and nothing lets an incident re-check itself on a
schedule.

Two facts from that design carry forward unchanged and shape every decision
below:

- **`coordinatorRef` is set ONLY on a conversation whose own entry point is
  a Coordinator** — addressed directly, or opened as a nested Coordinator's
  own root (`conversation-provenance`). An ordinary `capabilityRef` member
  has no `coordinatorRef` of its own, only `causedBy` naming its parent.
- **The invoke-time cycle guard already walks `causedBy` to the uncaused
  root**, collecting each ancestor's `coordinatorRef`, to decide whether an
  `invoke` would cycle the Coordinator graph. This change reuses that exact
  walk for a different question — not "would this cycle" but "which
  Coordinator does this caller act for."

Bundle wiring today (`k8s-bundle`, `prometheus-bundle`, `ha-bundle`) already
follows a four-condition qualification pattern before rendering a Pipeline:

- gated by an explicit flag
- every foreign reference a values-supplied name
- rendered only alongside its own profile
- defaulting off

Demo mode forces on the least-privileged route for `k8s-bundle` only, not for
`prometheus-bundle` or `ha-bundle`.

## Goals / Non-Goals

**Goals:**

- A release-wide chart posture that makes the Coordinator model
  out-of-the-box testable, with `pipelines` mode staying byte-identical to
  today.
- A self-heal mechanism built entirely from `invoke`, member-result
  routing, and one new, narrowly-scoped reach class — no new CRD, no new
  approval mechanism.
- Every decision traceable to an existing mechanism or an explicit,
  justified extension of one.

**Non-Goals:**

- Per-bundle mode granularity (settled in explore: release-wide only).
- A merged, cross-bundle capability (settled in explore: each bundle keeps
  its own privilege level, invoked as a distinct `agents[]` entry).
- Any CRD-level or CEL-level exclusivity between Pipeline and Coordinator
  wiring — this posture controls chart rendering only.
- A structured or enforced approval gate — escalation and reply are
  unchanged from `coordinated-agents`.

## Decisions

### D-A — Coordinator-owner reach resolves the caller's Coordinator by walking causedBy, reusing the cycle guard's own walk

A caller exercising the Coordinator-owner reach class is typically a
MEMBER, not a root — the reaper is invoked as an ordinary `capabilityRef`
entry of the cron-triggered root, so it carries `causedBy` naming that root
and no `coordinatorRef` of its own.

The manager resolves which Coordinator a caller acts for by reading the
caller's own `coordinatorRef` first. If empty, it walks the caller's
`causedBy` chain to the UNCAUSED root and reads that root's `coordinatorRef`
instead.

This is the SAME walk `coordination-loop`'s invoke-time cycle guard already
performs to collect ancestor Coordinators — no new field, no new index, a
second caller of an existing traversal.

Alternative rejected: requiring the caller itself to carry `coordinatorRef`
directly. This would make the reach class unusable by the reaper as
designed, since it is an ordinary member.

The only way to satisfy it would be inventing a new field propagating "acts
for Coordinator X" onto every member — a second source of truth beside what
`causedBy` and `coordinatorRef` together already state.

### D-B — close is widened to any open root of the caller's own Coordinator, excluding the caller's own ancestor

`aops-mcp-server`'s bound table gains one alternative on `close`: a caller
that resolves to a Coordinator (D-A) may also close any OPEN, UNCAUSED root
naming that same Coordinator — never a member, however shallow, and never
a root of a different Coordinator.

This is ADR D5's "nesting does not pool a budget across levels" instinct
applied to REACH instead of budget.

A budget is scoped to one Coordinator level so a wide tree doesn't starve a
sibling incident. The same scoping here stops a Coordinator-owner from
reaching into a tree it doesn't own.

One case D-A's walk makes newly possible, and this decision explicitly
excludes: the caller's OWN ancestor root — the uncaused root its `causedBy`
chain leads to. That root is itself an uncaused root of the caller's own
Coordinator by every other test in this bound.

Closing it would cascade to close the caller's own conversation mid-run
(`conversation-close`'s cascade rule). `list_open_roots` excludes it at the
listing step, and `close` refuses it even if named directly — the reaper
cannot end its own existence by closing the root that invoked it.

Two refusals follow from scoping to ROOTS, never members:

- A member is never a valid target, even one inside the caller's own
  Coordinator's tree. The ordinary `close` bound (caller, or a conversation
  it directly caused) already owns that case. Widening it too would let a
  Coordinator-owner reach arbitrarily deep into a tree it didn't cause —
  exactly what `invoke`'s `agents[]` bound and the cycle guard exist to
  prevent one layer over.
- A different Coordinator's root is never visible or closeable. Per-
  Coordinator isolation (ADR D5, D7) means one Coordinator's incidents are
  invisible to another's machinery by default.

### D-C — The chart renders AgentCapability directly per bundle, never both objects unconditionally

Considered: each bundle always renders BOTH a Pipeline and a standalone
AgentCapability, and `wiringMode` merely decides which one the
chart-rendered Coordinator references.

Rejected. It would leave an unused object in every install — an inert
AgentCapability under `pipelines` mode, an inert Pipeline under
`coordinator` mode — and it breaks the `pipelines`-mode byte-identical
requirement outright: today's install renders no AgentCapability at all.

Decided: each bundle's wiring component branches on `wiringMode` and
renders exactly the objects that mode needs.

This restates the EXISTING four-condition bundle-qualification pattern one
level down, substituting "an `AgentCapability` object" for "a `Pipeline`
object" in each clause — same flag, same values-supplied references, same
profile-gated rendering, same demo-mode exception forcing on only the
least-privileged route.

The chart-level Coordinator object itself — rendered once, release-wide —
is what gathers every bundle's `AgentCapability` into one `agents[]` list
and claims every bundle's source. That part is not a per-bundle decision:
it lives in the parent chart exactly as `pipelines:` does today for
install-declared wiring.

### D-D — The new reach class extends the existing coordinator token, not a sibling context

Considered: a third token-context family, parallel to
`coordinator:<name>:<conversation>` and `channel-reader:<channel>` —
something like `coordinator-owner:<name>:<conversation>`, injected only
into the reaper's runtime pod.

Rejected. The reaper IS an ordinary coordinator-class caller — it already
holds a `coordinator:<name>:<conversation>` token, because it's an ordinary
`agents[]` member invoked the ordinary way.

A sibling context would mean deriving and injecting a SECOND token per
reaper conversation, for one caller whose existing token context already
identifies everything the bound needs — which Coordinator it belongs to,
resolvable per D-A.

Decided: `list_open_roots` and the widened `close` are two more verbs the
EXISTING `coordinator:<name>:<conversation>` token reaches, gated by the
same per-verb bound table `aops-mcp-server` already has. No new context
string, no new derivation, no new pod-injected env var.

### D-E — The cron trigger reaches the reaper through the coordinating agent's own invoke, not a per-entry trigger field

Considered: giving each `agents[]` entry its own trigger — a claimed source
directly on the entry, bypassing the Coordinator's own inline agent.

Rejected. `coordinator-model` (ADR D2/D-B) places `signalSourceRefs` only
on the `Coordinator` object itself. An entry has no independent trigger,
and adding one is new CRD surface this change does not need.

Decided: the chart-rendered Coordinator claims the hourly `signals/cron`
source exactly as it claims every bundle's source — one more entry in the
same `signalSourceRefs` list.

Each admitted cron signal opens an ordinary conversation running the
Coordinator's OWN coordinating agent — the same capability that opens for
any other claimed source.

That agent's prompt instructs it to recognise an hourly cron signal and
respond by invoking the reaper's `agents[]` entry — the same judgement that
already routes every other signal to the entry that should handle it.

Consequence carried into D-A: the reaper's conversation is therefore a
MEMBER of the cron-triggered root, not a root itself, which is exactly the
case D-A's `causedBy` walk exists to resolve.

## Risks / Trade-offs

- **The reaper's re-check trusts the domain agent's own judgement.** Since
  the sweep invokes the ORIGINAL agent to re-verify rather than checking
  state directly, a domain agent that misjudges "healed" closes a root that
  is not actually resolved. Mitigation: this is the same trust the
  per-incident self-close already extends to that agent in the same turn —
  no new trust boundary, and the escalation path remains available if the
  domain agent instead reports the condition persists.
- **A wide coordinator-mode install with many bundles means one
  coordinating agent's prompt grows to cover every domain's routing
  decision**, including recognising the cron signal. Mitigation: out of
  scope for this change — the coordinating agent's prompt design is a
  template concern, not a contract one, and nothing here prevents
  splitting it later.
- **`list_open_roots` is a new read surface, even though narrowly scoped.**
  A compromised reaper conversation could enumerate every open incident of
  its own Coordinator (names, titles, briefs, phases — never transcripts).
  Mitigation: this is strictly narrower than what a human already sees in
  the console's tree view for the same Coordinator, and the reaper itself
  holds no domain tools to act on what it learns beyond `invoke`/`close`
  within its own Coordinator's `agents[]`.

## Migration Plan

1. Apply the two `coordinated-agents` CRDs first, if not already present —
   unchanged by this change.
2. `helm upgrade` with `global.agentops.wiringMode` unset or explicitly
   `pipelines`: byte-identical to today, verified by the render-diff test
   in `wiring-mode`'s spec.
3. Adopt `coordinator` mode: set the value, enable the bundles wanted, and
   the chart renders the Coordinator, each bundle's `AgentCapability`, and
   the reaper — no hand-authoring required.

Rollback: set `global.agentops.wiringMode` back to `pipelines` (or unset
it) and `helm upgrade`. The chart-rendered Coordinator and its
`AgentCapability` objects stop rendering.

Any conversation already open under them keeps its snapshot and ends by
budget, by the reaper's own sweep, or by hand — nothing cascades from the
wiring change itself.

## Open Questions

None — every question the drafting process surfaced (the reaper's trigger
mechanism, the ancestor-root exclusion) was resolved above rather than
deferred, since each would have changed a spec requirement or the task
breakdown.
