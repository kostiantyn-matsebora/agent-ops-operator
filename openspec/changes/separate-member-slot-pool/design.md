## Context

Capacity is decided in `ConversationReconciler`.

- `admit` counts live runtime pods against one number.
- `createRuntimePod` re-checks against a fresh list.
- `evictIdleWorker` frees a slot from the longest-idle pod.

Pods carry the label `LabelConversation` but no notion of role. The role lives
on the conversation record, where `CausedBy != nil` is a member
(`terminology.md`). See proposal.md for the motivation.

## Goals / Non-Goals

**Goals:**

- Two independent ceilings, both chart-set, both default 5.
- No behaviour change inside a pool: FIFO by creation time, pod-defined waiting set, idle eviction, the start deadline.

**Non-Goals:**

- A per-Coordinator or per-Pipeline budget. `maxAgents` stays per-level and untouched.
- A shared overflow between pools, or a total ceiling over both.
- A separate backlog bound. `maxQueuedConversations` keeps bounding all `Pending` conversations.

## Decisions

**D-1 Class by the conversation record, not by a pod label.**
A pod's class is the `CausedBy` field of the conversation its label names.

- The reconciler already does this lookup for eviction.
- A label stamped at creation would leave pods created before the upgrade unclassified.
- A pod whose conversation cannot be read counts toward the conversations pool. That is the conservative reading, since it never invents capacity for members.

**D-2 One classifier, one counting helper.**
`poolOf(conv)` returns `conversations` or `members`. `liveRuntimePods` stays pool-blind.
A helper splits its result into per-pool pod lists and per-pool caps.

`admit`, `createRuntimePod`, `evictIdleWorker`, `evictableCount` and `waitingFIFO`
take a pool argument rather than being duplicated. Every comparison against
`MaxActiveConversations` goes through the helper, so no path keeps the old single count.

**D-3 Eviction is pool-local.**
A conversation needing a slot evicts only an idle pod of its own pool.
Taking a root's idle pod for a member would re-couple the pools through the back door.

**D-4 Wake-up scales to both caps.**
On a pod deletion the reconciler wakes the oldest pending conversations of the
deleted pod's pool, capped at that pool's ceiling.

**D-5 Configuration.**
`maxActiveMembers: 5` in `chart/values.yaml` becomes `MAX_ACTIVE_MEMBERS`.

- Unset or invalid falls back to 5, as the conversations value does.
- The deprecated `MAX_RUNTIMES` alias is unchanged and sets only the conversations pool.

**D-6 Introspection.**
`/status` keeps its current slot fields, now describing the conversations pool.
It gains a sibling object for members with the same `inUse`, `max` and `waiting` shape.
That is additive, so existing readers keep working.

The Prometheus gauges gain a `pool` label rather than new metric names.
The unlabelled series are retired in the same change, since a sum across pools would mislead.
The console and the alert text read the pool.

**D-7 Nested coordinators do not deadlock the members pool, and a test pins it.**
A member that is itself a Coordinator holds a members slot like any other member.

It cannot starve its own children.
`invoke` returns at once and never waits (`docs/concepts.md`, the invoke/result/close loop).

1. The parent's run ends.
2. Its conversation has nothing inflight and nothing queued, so its pod is EVICTABLE.
3. A child needing a slot in a full members pool evicts the idle parent. D-3 keeps eviction inside the pool, which is where both live.
4. The parent gets a pod again when a child's result arrives as an input, waiting FIFO like any other.

The pool can only stall if every slot holds a BUSY run.
A run is bounded by the Coordinator's `maxTurns` and `deadline`.
A chain of six nested members against a ceiling of 5 therefore still completes.

Alternative: a depth-aware reservation that keeps a slot free for the deepest level.
Rejected, because nothing deadlocks without it and it would add the pooled
budgeting `invariants.md` forbids.

Residual risk: a runtime whose run blocks waiting on a member's result holds its
slot busy until its own deadline. The shipped runtimes do not do this.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Total pods can double to 10, exhausting node resources | State it as breaking in the changelog and docs. The ephemeral-storage guidance multiplies both values |
| A member pod's conversation record is deleted before its pod | Counted against conversations (D-1). A transient over-count, never an over-admission |
| The per-conversation lookup adds store reads | It is the same read eviction already does, bounded by the ceilings. The fresh re-check in `createRuntimePod` stays the last word |
| The metric relabel breaks dashboards | Listed in the changelog. The shipped alert is updated in the same change |

## Migration Plan

Upgrade with no values set: conversations keep 5 and members gain their own 5.

An install that wants to hold today's total lowers `maxActiveConversations` and
`maxActiveMembers` between them. Each has a minimum of 1.

Rollback is reverting the chart. The older manager ignores the env var.
