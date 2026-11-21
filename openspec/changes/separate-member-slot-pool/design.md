## Context

Capacity is decided in `ConversationReconciler`: `admit` counts live runtime
pods against one number, `createRuntimePod` re-checks against a fresh list and
`evictIdleWorker` frees a slot from the longest-idle pod. Pods carry the label
`LabelConversation` but no notion of role; the role lives on the conversation
record (`CausedBy != nil` is a member — `terminology.md`). See proposal.md for
the motivation.

## Goals / Non-Goals

**Goals:**
- Two independent ceilings, both chart-set, both default 5.
- No behaviour change inside a pool: FIFO by creation time, pod-defined waiting set, idle eviction, the start deadline.

**Non-Goals:**
- A per-Coordinator or per-Pipeline budget (`maxAgents` stays per-level and untouched).
- A shared overflow between pools, or a total ceiling over both.
- A separate backlog bound: `maxQueuedConversations` keeps bounding all `Pending` conversations.

## Decisions

**D-1 Class by the conversation record, not by a pod label.** A pod's class is
the `CausedBy` field of the conversation its label names. The reconciler already
does this lookup for eviction. A new pod label would leave pods created before
the upgrade unclassified, and the class is not a fact a pod can outlive.
Alternative: a label stamped at creation — rejected for the migration gap. A
pod whose conversation cannot be read counts toward the conversations pool
(the conservative reading: it is never invented capacity for members).

**D-2 One classifier, one counting helper.** `poolOf(conv)` returns
`conversations` or `members`; `liveRuntimePods` stays pool-blind and a helper
splits its result into per-pool pod lists and per-pool caps. `admit`,
`createRuntimePod`, `evictIdleWorker`, `evictableCount` and `waitingFIFO` take a
pool argument rather than being duplicated. Every place that compared against
`MaxActiveConversations` goes through the helper, so no path keeps the old
single count.

**D-3 Eviction is pool-local.** A conversation needing a slot evicts only an
idle pod of its own pool. Taking a root's idle pod for a member would
re-couple the pools through the back door.

**D-4 Wake-up scales to both caps.** On a pod deletion the reconciler wakes the
oldest pending conversations of the deleted pod's pool, capped at that pool's
ceiling, instead of one list capped at one number.

**D-5 Configuration.** `maxActiveMembers: 5` in `chart/values.yaml` →
`MAX_ACTIVE_MEMBERS`. Unset or invalid falls back to 5, as the conversations
value does. The deprecated `MAX_RUNTIMES` alias is unchanged and sets only the
conversations pool.

**D-6 Introspection.** `/status` keeps its current slot fields (now the
conversations pool) and gains a sibling object for members with the same
`inUse`/`max`/`waiting` shape — additive, so existing readers keep working. The
Prometheus gauges gain a `pool` label rather than new metric names; the existing
unlabelled series are retired in the same change since a sum across pools would
mislead. Console and the alert text read the pool.

**D-7 Nested coordinators do not deadlock the members pool, and a test pins it.**
A member that is itself a Coordinator is a member like any other and holds a
members slot. It cannot starve its own children, because `invoke` returns at
once and never waits (`docs/concepts.md`, the invoke/result/close loop): the
parent's run ends, its conversation has nothing inflight and nothing queued, and
its pod is EVICTABLE. A child needing a slot in a full members pool evicts an
idle parent (D-3 keeps eviction inside the pool, which is where both live). The
parent gets a pod again when a child's result arrives as an input, waiting FIFO
like any other. The pool can only stall if every slot holds a BUSY run, and a run
is bounded by the Coordinator's `maxTurns`/`deadline`. A chain deeper than the
ceiling (six nested members against a ceiling of 5) therefore still completes.
Alternative: a depth-aware reservation (keep a slot free for the deepest
level) — rejected, nothing deadlocks without it and it would add the pooled
budgeting `invariants.md` forbids. The residual risk is a runtime whose run
blocks waiting on a member's result; it holds its slot busy until its own
deadline, and the test below uses the shipped runtimes' behaviour.

## Risks / Trade-offs

- [Total pods can double to 10, exhausting node resources] → state it as breaking in the changelog and docs; ephemeral-storage guidance multiplies both values.
- [A member pod's conversation record is deleted before its pod] → counted against conversations (D-1), a transient over-count, never an over-admission.
- [Per-conversation lookup in the count adds store reads] → the same read eviction already does, bounded by the ceilings; the fresh re-check in `createRuntimePod` stays the last word.
- [Metric relabel breaks dashboards] → listed in the changelog; the shipped alert is updated in the same change.

## Migration Plan

Upgrade with no values set: conversations keep 5, members gain their own 5. An install
that wants to hold today's total lowers `maxActiveConversations` and `maxActiveMembers`
between them (each minimum 1). Rollback is reverting the chart: the env var is ignored by the older manager.
