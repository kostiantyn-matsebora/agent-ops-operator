## Context

- Manager operations run inside the request. `OpQueue` (`internal/chat/ops.go`)
  is the only queue. It is in memory, per channel type, claimed by the leader.
  Recovery is the reconciler re-deriving an op. See `proposal.md`.
- The state store is a generic contract (`internal/state`) with a `kube`
  backend. The `nats` backend, and its server bundle, arrive with
  `nats-state-store-default` (PR #471, final verification).
- Only `internal/state/nats` may import `nats.go`, enforced by a static test.
- The manager mounts no volume and reads no secret (`invariants.md`).
- The runtime protocol (`/work*`) and the adapter protocol are contracts the
  change must not edit (`runtime-spec.md`, `channel-spec.md`).
- `OpQueue`'s dedup rule comes from the 2026-08-13 incident. A failure must
  never be recorded as a success.

## Goals / Non-Goals

**Goals:**

- Accept work durably, run it later on any replica, record what became of it.
- Parallel across keys, FIFO within a key.
- One retry owner, with backoff and a dead-letter record.
- A bus that stands alone. A `kube`-store install can run it.

**Non-Goals:**

- The console task UI, retry and dismiss on dead letters, async `invoke`.
- Multi-task atomicity. A task is one verb on one target.
- Exactly-once delivery. Delivery is at least once and handlers are idempotent.
- Cross-cluster topologies, and an out-of-process bus application.

## Decisions

### D-1: Its own package, its own configuration

`platform/manager/internal/commandbus/` holds the contract. Backends live in
`commandbus/nats/` and `commandbus/memory/`. The static import test names both
`internal/state/nats` and `commandbus/nats` as the only packages that may
import `nats.go`. Neither imports the other.

The bus reads its own variables (`COMMANDBUS_BACKEND`, `COMMANDBUS_NATS_URL`,
`COMMANDBUS_NATS_USER`, `COMMANDBUS_NATS_PASSWORD`), its own Secret and its own
NetworkPolicy rule. It never reads `NATS_*`.

| Alternative | Rejected because |
|---|---|
| A capability inside `state.Stores` | Ties async to the store backend. A `kube` install would lose it or need a second implementation |
| One shared connection package | Turns a coincidence (both use NATS) into a coupling of credentials and dial paths |

An adopter may still point both at one server. That is a deployment choice. The
bus prefixes its buckets so nothing collides.

### D-2: The ledger drives everything, the transport only stores and wakes

Order, lease and backoff all live in the ledger, never in the transport.

| Concern | Where it lives |
|---|---|
| Lease (claim token, lease-until) | the task record, written by compare-and-swap on its revision |
| Next eligible time after a failure | `notBefore` on the task record |
| Per-key order | claim-time rule over the ledger's ordered feed |
| Wake-up of an idle consumer | the ledger's change feed |

This is what lets a claim on one replica be completed on another. A message
object's acknowledgement cannot do that.

The `nats` backend therefore needs only JetStream KV buckets and their watches:
`tasks`, `ids` (command id to current task) and `dead`. It needs no delivery
stream.

**Why the ordering rule is safe under a stale view.** A consumer claims a task
only when no earlier task of its key is unsettled. Three facts make that sound:

1. The change feed is ordered. Seeing task 2 means task 1 was already seen.
2. Settled is terminal. A stale view can only call a task unsettled, never
   settled by mistake.
3. The claim itself is a compare-and-swap on the task's own revision.

So the worst a stale view does is delay a claim.

| Alternative | Rejected because |
|---|---|
| Partitioned subjects, one consumer each | A slow task stalls unrelated keys that share a partition. Partition count would leak into the contract |
| Message acks as the lease | Cannot complete on another replica |

### D-3: A library is allowed under the transport only if a spike passes

The layer in D-2 is ours. A message library (Watermill with its NATS
JetStream module is the candidate) would serve only as transport. The first
task is a spike against the test-only `nats-server` the state backend already
vendors. It must show four things:

1. A publish returns only once JetStream has stored it, and a repeated id is
   suppressed.
2. A claim on one replica completes on another, and an expired lease returns
   the task.
3. Delayed redelivery and a bounded attempt count end in the dead record.
4. The `nats.go` version the library needs (1.52 against the state backend's
   1.39.1) passes the state backend's conformance suite unchanged.

If any check fails, or the library adds more than it saves once D-2 owns the
lease, the `nats` backend uses `nats.go` KV directly, as the state backend does.
That fallback needs no spec change and no task change beyond the transport file.

### D-4: Command id, task uid, and the dedup states

The **command id** is the dedup key and is derived by the producer. The **task
uid** is unique per attempt record.

```
queued ─claim─▶ running ─ok──────────────▶ succeeded   (id suppressed for the settled window)
   ▲               │ error (retryable) or lease expiry
   └─ retrying ◀───┘   id stays pending, so a re-derive dedups
                   │ attempts exhausted, or a terminal code
                   ▼
                 dead ─ parked for a cool-off ─▶ id released, resubmittable
queued past its TTL ─▶ expired   (settled for ordering, never runs)
```

- A submit finds the id in `ids` and is suppressed in every state except
  released.
- A sweep releases a parked id after its cool-off. The dead task's record stays
  in history.
- `expired` counts as settled for ordering, so a task that never ran cannot
  block its key.

### D-5: Queue declarations, with defaults the chart can change

Numbers below are starting values. The bench and the system lanes confirm or
change them. Each is a chart value.

| Queue | Key | Attempts | Backoff | TTL | Notes |
|---|---|---|---|---|---|
| `conv` | root conversation | 5 | 1s to 30s, jitter | none | in-flight cap per invocation paces a bulk batch |
| `in.signal` | source + `initiationId` | 5 | 1s to 30s, jitter | none | |
| `out.<type>` stable kinds | conversation × channel | 8 | 5s to 5m, jitter | none | ensure-topic, run reply, input card, close-topic |
| `out.<type>` ephemeral sends | conversation × channel | 2 | 5s | 2m | acks and notices |
| `out.<type>` delete-conversation | conversation × channel | 3 | 5s | finalizer grace | never parked |

Each queue also declares its dedup window, its park cool-off and the
per-invocation in-flight cap. A kind may override a queue's attempts and
backoff.

### D-6: Retry is derived from the code, and `precondition_failed` is terminal

The vocabulary is closed: `not_found`, `precondition_failed`, `forbidden`,
`conflict`, `unavailable`, `invalid_command`, `internal`.

| Code | Behaviour |
|---|---|
| `conflict`, `unavailable`, `internal` | retry with backoff |
| `not_found`, `precondition_failed`, `forbidden`, `invalid_command` | straight to dead |

A consumer never chooses. An unknown code is recorded as `internal`.

`precondition_failed` is terminal on purpose. A refused `/exit` mid-run must not
become a surprise release minutes later. An adapter's reported error string
maps to `unavailable`, so a rate-limited surface backs off.

`delete` of a conversation that is already gone succeeds and does not go dead
on `not_found`. Deleting something absent is the outcome the caller wanted.

### D-7: A cascade is enqueued at accept, and the ancestor re-checks

The accept step reads the `causedBy` tree and enqueues one task per member.

- Members go before the parent, all on the root's key.
- The order is visible at once in the ledger.
- It is simpler to reason about than a parent that enqueues its own members at
  run time.

A tree can change between accept and execution, so the ancestor's task
re-checks its descendants when it runs.

- A dead member task still releases its key.
- Without the re-check, the ancestor could then close over an open member.
- The ancestor either closes any open descendant itself or fails
  `precondition_failed`.

| Alternative | Rejected because |
|---|---|
| Expand at run time | The parent's row shows no work until it runs, and a crash between expansion and enqueue needs its own recovery |

### D-8: `escalate` is two atomic tasks, and the result append has its own verb

| Case | Tasks |
|---|---|
| Uncaused root | one `escalate` task: a patch setting `escalatedAt` and the bounded message, a no-op when already set |
| Caused member | a cascading `close`, then `append-result`, one invocation, one key (the root's) |

`append-result` is a new verb on `conv`, keyed by the parent's root. Its dedup
id derives from the task, not the clock. The current id embeds
`time.Now().UnixNano()`, so a redelivery would append the result twice.

The inline calls from `/work/done` (the second-miss escalation) keep calling
`Router.Escalate` directly. That path stays synchronous (D-12).

### D-9: Accept is cheap and stateless, process does the work

The accept step runs, in order:

1. Authentication, adapter or state scope, body shape.
2. Unknown channel or source.
3. For signals: the Wired reason and the attribute-filter reason.
4. For signals: the activity "signal received" receipt.
5. For coordinator verbs: the move stamp.
6. For inbound chat: the thread-to-conversation lookup that yields the key.
7. Mint the `invocationId`, enqueue durably, reply `202` with
   `{invocationId, taskIds}`, added to whatever the endpoint returned before.

Refusals that depend only on a cheap read stay immediate. `delete` of a
conversation that is not Closed stays a `409` at accept.

Inbound dedup: a signal keeps its `initiationId`. A channel message uses an
optional `Idempotency-Key` header, and absent that a key derived from channel,
thread, text and sender. No adapter has to change.

### D-10: `OpQueue` keeps its surface, with bus-backed internals

`Enqueue*`, `Claim`, `Complete`, `Settled`, `Pending` and `Stats` stay. The
router, reconciler and handlers do not change.

- **Dedup** follows D-4. A stable op id is suppressed while pending, retrying,
  parked or settled. A failure never suppresses its own recovery.
- **`Complete` still runs its handlers synchronously** before the `200`:
  `finishEnsureTopic`, the delivered and undelivered markers, the archived
  marker. The thread binding they write is what inbound messages resolve
  through. A handler that fails releases the id, as today.
- **`Pending(id)` and `Settled(id)`** run on every reconcile pass. They read a
  watch-fed local mirror of pending and settled ids, never the server.
- **Ephemeral sends** carry the short TTL in D-5.
- **In-process providers** become a consumer consuming `out.<type>`.
- **`/api/queues`** keeps working from the per-queue statistics.

### D-11: The leader proxy narrows and the claim placeholder retires

Any replica serves `/channel/ops` and `/channel/ops/{id}/done`, so the leader
proxy exempts them along with `/healthz`, `/state/tasks*` and
`/state/dead-letters`. Accept endpoints still proxy to the leader. That is
conservative and can narrow later.

`status.threads[].claim` is no longer written, and a value found there is
ignored. The lease defaults to five minutes through the existing
`claimStalenessSeconds`, so the `reclaimAfterSeconds` an adapter receives does
not change.

### D-12: What stays synchronous, and why

| Operation | Reason |
|---|---|
| `invoke` | the reply carries the member and the budget reservation. An accepted-then-failed invoke would still count as a move |
| `/work`, `/work/done`, `/work/context` | the runtime contract. Move-per-turn runs inside `/work/done` before the run is recorded, so a retried report is judged again and the corrective append lands once |
| `/channel/ops`, `/channel/ops/{id}/done` | the consumer end of `out.*` |
| reads, streams, sessions, adapter state and status, `/activity`, mark read and unread | no work to defer, or the caller needs the result |

The budget-exceeded close is enqueued as a `conv` close task after the run is
recorded. Task 5.4 confirms the call site before the change is finished.

### D-13: The chart

`commandBus.backend` is `nats` or `memory`, independent of
`stateStore.backend`. A guard fails every contradiction and names the value.

- `nats` requires exactly one of a rendered bundle or `commandBus.nats.url`.
  A URL needs its own credential.
- The default renders a bundle, with the same shape as the state bundle: three
  servers, a volume each, a PodDisruptionBudget, a chart-generated credential.
  Whether the existing subchart can be instantiated a second time under an alias
  is settled in task 8.1. If it cannot, the bus gets its own template.
- `memory` is refused with more than one manager replica.
- The manager reports not ready while the bus is unreachable.

### D-14: Tests are three tiers, and the conformance suite is the first

| Tier | Decides |
|---|---|
| Unit | the conformance suite against `memory` and against an in-process `nats-server`, the bench, handlers against `memory`, the chart's render and guard tests |
| System | what a cluster decides: the bundle, credential projection, the NetworkPolicy, a claim on one replica completed on another, restart durability, ordering under parallel consumers, backoff into a dead letter |
| End-to-end | full flows through the real HTTP surface with the task stream observed, in the full tier |

The end-to-end flows are listed in the tasks. The repo's docs-task guard fixes
unit, system and documentation as the last three sections, so the end-to-end
section sits just before unit.

## Risks / Trade-offs

- **A `200` now means accepted** → the semantic change is named BREAKING in the
  changelog, `docs/contracts.md` states it per endpoint, and a task carries the
  outcome.
- **A dead task is invisible to an agent that moved on** → every dead task
  leaves a dead-letter record and a metric. Only `invoke` is kept synchronous,
  because it is the verb whose failure the agent must see.
- **Head-of-line delay on a key** → bounded by attempts and backoff, then the
  key is released. Other keys are never affected.
- **One lane per coordination tree** → fine for the current sizes. Members could
  move to their own keys later.
- **A new `nats.go` version for the state backend** → the state suite re-runs
  against it in the spike, before anything else is built.
- **Reconcile hot-path cost** → the local mirror of pending and settled ids.
- **The ledger is a second state home** → declared bounded and lossy. A lost
  ledger record never loses a conversation, and the reconciler re-derives owed
  ops.
- **A rate-limited surface still sees waves of retries** → jitter spreads them.
  A per-queue pause is deferred and noted in the open questions.
- **Two retry loops (bus and reconciler)** → the id stays pending while the bus
  retries, so a re-derive is suppressed. After dead, a cool-off prevents an
  instant resubmit.

## Migration Plan

1. Upgrade the chart. A default install renders the bus bundle. The manager
   starts when the bus is reachable.
2. The ledger starts empty. Ops owed at the moment of upgrade are re-derived by
   the reconciler, as they are after any restart today.
3. Requests in flight at the upgrade finish under the old manager. New ones
   are accepted by the new one.

Rollback: select `commandBus.backend: memory` on one replica, or the previous
chart. Accepted but unprocessed tasks are lost on the rollback. The docs name
this plainly.

## Open Questions

- Starting values for backoff and attempts. Settled from the bench and the
  system lanes. No spec depends on them.
- Dead-letter retention caps (age and size). Chart values with documented
  defaults.
- A per-queue pause on consecutive failures. Deferred. It adds a state, not a
  contract change.
