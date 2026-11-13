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

The bus reads its own variables (`COMMANDBUS_BACKEND`, `COMMANDBUS_NATS_URL`, `COMMANDBUS_NATS_USER`, `COMMANDBUS_NATS_PASSWORD`). It never reads `NATS_*`. Where it has a server of its own, a dedicated bundle or an external one, it has its own Secret and NetworkPolicy rule.

Where it shares the state store's server (D-13) the chart points those variables at the state store's Service and credential, and only the buckets stay the bus's own.

| Alternative | Rejected because |
|---|---|
| A capability inside `state.Stores` | Ties async to the store backend. A `kube` install would lose it or need a second implementation |
| One shared connection package | Turns a coincidence (both use NATS) into a coupling of credentials and dial paths |

An adopter may still point both at one server. That is a deployment choice. The
bus prefixes its buckets so nothing collides.

### D-2: A queue is an append-only log, and delivery state is derived from it

**No entry is ever modified.** A message is appended once, when it is
submitted. Everything that later happens to it is a further append, never a
rewrite.

| Concern | Where it lives |
|---|---|
| The message | one immutable entry, appended once. Its identity is the producer's command id, made before the append. It says nothing about order |
| Order | the position the log gives the entry on append. FIFO is the log's own. The message carries no order or sequence field |
| Claim, lease, failure, retry, settle | further entries appended to the same log, never changes to the message |
| Current state of a message | derived by folding its own entries: the last one decides |
| Consumer position | none. A consumer keeps no cursor. A message nobody claimed or acknowledged is simply offered again |

The `nats` backend is one JetStream stream per queue family, with subjects
`<queue>.<key>.msg` and `<queue>.<key>.ev`. Nothing in it is ever updated.

**Consumed messages are deleted, unconsumed ones are kept for 7 days.** When a message settles as succeeded, the server removes it and its entries, as a regular queue drops what was consumed.

A message never consumed is retained for the stream's age limit (7 days by default) and then trimmed, so it expires. The bus performs no other delete and never touches an unsettled entry.

A command id's duplicate suppression after settling is an in-memory mark each replica keeps for the queue's dedup window (the server's duplicate window is one stream-wide value and cannot hold per-queue windows).

Physical removal follows a short grace (30s by default) so a replica catching up and the change feed see no gap.

Every replica folds the same stream, so each sets the mark when it applies the settle entry, and a replica that was running then holds it.

A replica that starts after the entries were removed holds none, so a duplicate submitted to it inside the window is accepted. That is the at-least-once delivery D-4 already names, and the handler's idempotence absorbs it.

The mark is a best-effort shortcut for the common case, never the only guard.

- **Exclusive claim without a rewrite.** A claim is an append of a `claimed`
  entry to the key's `ev` subject, conditional on that subject's last sequence
  (JetStream's expected-last-subject-sequence). Two consumers racing for one
  key cannot both append, so a lost race is a rejected append, never a lost
  update.
- **Per-key order.** A consumer may claim an entry only when every earlier
  entry of its key is settled. Settled is terminal, so a stale view can only
  delay a claim.
- **A lapsed lease needs no write.** A running entry whose lease has passed
  reads as retryable when folded. The next claim appends with the next attempt.
- **Wake-up** is the stream's own change feed.
- **A dead entry** is appended to a separate `dead` log with its own limits, the
  bound on dead-letter retention.

Every consumer folds the log into a local mirror, so a claim on one replica
completes on another.

| Alternative | Rejected because |
|---|---|
| One mutable record per message, rewritten on claim and settle | Two writers contend on one record. Measured: a claim landing between a submit's two writes failed the submit with NATS 10071 |
| Partitioned subjects, one consumer each | A slow task stalls unrelated keys that share a partition |
| Message acks as the lease | Cannot complete on another replica |

### D-3: A library is allowed under the transport only if a spike passes

The layer in D-2 is ours. A message library (Watermill with its NATS
JetStream module is the candidate) would serve only as transport. The first
task is a spike against the test-only `nats-server` the state backend already
vendors. It must show four things:

1. A publish returns only once JetStream has stored it, and a repeated id is
   suppressed.
2. A claim on one replica completes on another, and a lapsed lease returns
   the task.
3. Delayed redelivery and a bounded attempt count end in the dead record.
4. The `nats.go` version the library needs (1.52 against the state backend's
   1.39.1) passes the state backend's conformance suite unchanged.

If any check fails, or the library adds more than it saves once D-2 owns the
lease, the `nats` backend uses `nats.go` KV directly, as the state backend does.
That fallback needs no spec change and no task change beyond the transport file.

### D-4: Command id, and the dedup states

The **command id** is the dedup key and is derived by the producer before the
append. The state below is derived by folding the entry's own appended entries.

```
queued ─claim─▶ running ─ok──────────────▶ succeeded   (id suppressed for the settled window)
   ▲               │ error (retryable) or lease lapse
   └─ retrying ◀───┘   id stays pending, so a re-derive dedups
                   │ attempts exhausted, or a terminal code
                   ▼
                 dead ─ parked for a cool-off ─▶ id released, resubmittable
queued past its TTL ─▶ expired   (settled for ordering, never runs)
```

- A submit finds the id's latest entry and is suppressed while the latest task is
  queued, running, retrying, parked, or succeeded within the dedup window.
- It is not suppressed once the task succeeded past the window, is dead past its
  cool-off, or expired.
- A parked id is released by the passage of its cool-off, with no write. The dead
  entry stays in the `dead` log until its limits remove it.
- `expired` counts as settled for ordering, so a task that never ran cannot
  block its key.

### D-5: Queue declarations, with defaults the chart can change

Numbers below are starting values. The bench and the system lanes confirm or
change them. Each is a chart value.

| Queue | Key | Attempts | Backoff | TTL | Notes |
|---|---|---|---|---|---|
| `conv` | root conversation | 5 | 1s to 30s, jitter | none | in-flight cap per invocation paces a bulk batch |
| `in.signal` | source + `initiationId` | 5 | 1s to 30s, jitter | none | |
| `out.<channel>` stable | conversation | hand-over | none | queue none, delivery 10m | run reply, input card, farewell |
| `out.<channel>` ephemeral | conversation | hand-over | none | queue 2m, delivery 2m | acks and notices |
| `out.<channel>` topic | conversation | hand-over | none | delivery 5m | ensure-topic, close-topic |
| `out.<channel>` delete-conversation | conversation | hand-over | none | queue 5m, delivery 5m | never parked |

`conv` and `in.signal` are lease-and-retry queues. The `out.<channel>` kinds are
hand-over queues (D-17): the bus does not retry them, because the adapter
retries against its own surface until the delivery TTL, so they declare no
attempts and no backoff.

Each queue also declares its dedup window, its park cool-off and the
per-invocation in-flight cap. Each `out.<channel>` kind is a complete
declaration of its own.

### D-6: Retry is derived from the code, and `precondition_failed` is terminal

The vocabulary is closed: `not_found`, `precondition_failed`, `forbidden`,
`conflict`, `unavailable`, `invalid_command`, `internal`.

| Code | Behaviour |
|---|---|
| `conflict`, `unavailable`, `internal` | retry with backoff |
| `not_found`, `precondition_failed`, `forbidden`, `invalid_command` | straight to dead |

A consumer never chooses. On a lease-and-retry queue a lapsed lease counts as an attempt that failed with `unavailable`, so a silent consumer is retried and then dead-lettered.

On a hand-over queue a lapse or a reported failure is final and never retried (D-17). An unknown code is recorded as `internal`.

`precondition_failed` is terminal on purpose. A refused `/exit` mid-run must not
become a surprise release minutes later. An adapter's reported
error on a hand-over queue is final, so a rate-limited surface is ridden out by
the adapter inside the delivery TTL, not by the bus.

`delete` of a conversation that is already gone succeeds and does not go dead
on `not_found`. Deleting something absent is the outcome the caller wanted.

### D-7: A cascade is enqueued at accept, and the ancestor re-checks

The accept step reads the `causedBy` tree and enqueues one task per member.

- Members go before the parent, all on the root's key.
- The order is visible at once in the log.
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

### D-8: `escalate` is one idempotent task

| Case | Work inside the one `escalate` task |
|---|---|
| Uncaused root | a patch setting `status.escalatedAt` and the message, a no-op when already set |
| Caused member | the cascading close with the message as its reason, and the result appended to the parent |

The task's command id is `escalate:<conversation>`, so a redelivery is
suppressed or replays the same effect. Each step is conditional on the state it
finds, so a repeat changes nothing.

A split into a `close` task then an `append-result` task was planned and not
built. One task per conversation is enough, since both steps already guard
themselves and a second task would add a write on the key.

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

### D-10: `OpQueue` keeps its enqueue surface, with bus-backed internals

`Enqueue*`, `Settled`, `Pending` and `Stats` stay. The old `Claim` becomes the
three adapter-facing calls of D-17, `Read`, `Acknowledge` and `Complete`. The
router, reconciler and handlers do not change.

- **Dedup** follows D-4. A stable op id is suppressed while pending, parked or
  settled. A failure never suppresses its own recovery.
- **`Complete` still runs its handlers synchronously** before the `200`:
  `finishEnsureTopic`, the delivered and undelivered markers, the archived
  marker. The thread binding they write is what inbound messages resolve
  through. A handler that fails fails the message. That is final, and the
  reconciler re-derives what is still owed once the kind's park cool-off passes.
- **`Pending(id)` and `Settled(id)`** run on every reconcile pass. They read a
  watch-fed local mirror of pending and settled ids, never the server.
- **Ephemeral sends** carry the short TTL in D-5.
- **One queue per Channel resource**, `out.<channel>`. An adapter drains the
  queues of every Channel that names it, so `/channel/ops?adapter=` is unchanged.
- **In-process providers** become a consumer consuming `out.<channel>`.
- **`/api/queues`** keeps working from the per-queue statistics.

### D-11: The leader proxy narrows and the claim placeholder retires

Any replica serves the consumer end of `out.*`: `GET /channel/ops`,
`GET /channel/ops/stream`, `POST /channel/ops/{id}/ack` and
`POST /channel/ops/{id}/done`. The leader proxy exempts them along with
`/healthz` and everything under `/state/`. Accept endpoints still proxy to the
leader. That is conservative and can narrow later.

`status.threads[].claim` is no longer written, and a value found there is ignored. No lease is placed on a channel op at all: an acknowledged message is the adapter's, and its delivery TTL is the only clock (D-17).

The old `reclaimAfterSeconds` field is gone from the op, replaced by the acknowledgement reply. `claimStalenessSeconds` is inert.

### D-12: What stays synchronous, and why

| Operation | Reason |
|---|---|
| `invoke` | the reply carries the member and the budget reservation. An accepted-then-failed invoke would still count as a move |
| `/work`, `/work/done`, `/work/context` | the runtime contract. Move-per-turn runs inside `/work/done` before the run is recorded, so a retried report is judged again and the corrective append lands once |
| `/channel/ops`, `/channel/ops/{id}/ack`, `/channel/ops/{id}/done`, `/channel/ops/stream` | the consumer end of `out.*` |
| reads, streams, sessions, adapter state and status, `/activity`, mark read and unread | no work to defer, or the caller needs the result |

The budget-exceeded close is enqueued as a `conv` close task after the run is
recorded, at both of its call sites.

### D-13: The chart

`commandBus.backend` is `nats` or `memory`, independent of
`stateStore.backend`. A guard fails every contradiction and names the value.

- The default, when the state store is itself on `nats`, shares that bundle: the
  bus connects to its client Service with its credential, in buckets of its own.
  Nothing is configured and nothing second renders. With the state store on
  `kube` there is no server to share, so the default renders the dedicated bundle
  below.
- `commandBus.nats.enabled: true` renders a dedicated bundle with the same shape
  as the state bundle: three servers, a volume each, a PodDisruptionBudget, a
  chart-generated credential. The existing subchart cannot be instantiated twice
  under an alias, so the bus has its own template (task 8.1).
- `commandBus.nats.url` points at an external server and needs its own credential
  from `commandBus.nats.auth.user` and `password`, or `commandBus.nats.auth.existingSecret`.
- The dedicated bundle and the URL are mutually exclusive.
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

### D-15: Thread entries have one fan-out point

One function records a thread entry and enqueues it to the queue of every bound
channel. Bound channels are the Conversation's snapshot of the Pipeline or
Coordinator wiring, less the surface that already displayed the entry.

- Inputs, run replies, escalations and notices all pass through it, so the order
  in which they are enqueued is the order they happened.
- It replaces the separate producers in the reconcile pass (`DeliverInputs`,
  `deliverRunReplies`, `deliverEscalation`, the reopen notice). The reconciler
  remains the backstop and calls this one function.
- It supersedes the stopgap that flushed run replies before later inputs.

A queue preserves enqueue order and nothing more. Measured live, the nudge was
enqueued before the analysis it reacted to, so it reached the console first.

| Alternative | Rejected because |
|---|---|
| Order by timestamp inside each producer | Every new producer must remember the rule. The stopgap did this for two |
| A global queue for all channels | One slow channel delays every other |

### D-16: The retained console feed (withdrawn)

Two shapes were built for the console and withdrawn, each found fragile live. The first gave it claim, lease and acknowledgement like Telegram: a dead connection's claim stranded ops for a whole lease.

The second kept a retained log and a cursor the client resumed from: it needed a replay window, an event id and render-once bookkeeping, and a push stream that was the only way to a message made one lost frame invisible.

Neither exists. The console is the receipt-only case of the one contract in D-17, and nothing in the bus keeps a cursor, a sequence or a retained feed.

### D-17: Adapters own delivery: get, ack, then done or failed

Every channel adapter delivers under one contract. It is generic: a new adapter
needs no manager or bus code and no inventory entry, and delivery TTLs are
declared per message kind, never per adapter.

| Step | Call | Effect |
|---|---|---|
| GET | `GET /channel/ops?adapter=&contract=&wait=` | Reads the next message of the adapter's channel queues and changes nothing. A message read by an adapter that dies before acknowledging is offered again, since no delivery had begun |
| ACK | `POST /channel/ops/{id}/ack?adapter=` | The hand-over. From it the message is the adapter's: the manager never sends it again and its delivery TTL starts |
| DONE or FAILED | `POST /channel/ops/{id}/done` with `{threadId?, error?}` | The adapter's report after delivering to its surface with its own retries. An `error` is FAILED |

```
GET  /channel/ops?adapter=telegram&contract=2&wait=25
200  {"id":"send:conv-1:tg-ops:run-7","channel":"tg-ops","kind":"send","threadId":"42","message":{...}}

POST /channel/ops/send:conv-1:tg-ops:run-7/ack?adapter=telegram
200  {"ok":true,"deliveryTtlSeconds":600,"consumed":false}

POST /channel/ops/send:conv-1:tg-ops:run-7/done        {}                       # done
POST /channel/ops/topic:conv-1:7c1e:tg-ops/done        {"threadId":"1187"}      # done, with the result
POST /channel/ops/send:conv-1:tg-ops:run-7/done        {"error":"chat not found"} # failed
```

- **Acknowledge answers** `404` when the message is gone (consumed or expired) and
  `409` when it is held but not offered (behind an unfinished message of its
  conversation, backing off, or finished). A repeated acknowledgement within the
  TTL returns the same reply, on any replica, so a retry hands over nothing twice.
- **No report by the TTL** fails the message, by the adapter itself and by the
  manager as a backstop when the adapter is gone. **Failed is final.** The bus
  never redelivers, and what is still owed is re-derived by the reconciler only
  after the kind's park cool-off.
- **The acknowledgement is always between aops components**, adapter to manager,
  never from the external service. A result only the external service can
  produce, such as the topic id of a Telegram `ensure-topic`, travels in the DONE
  report.
- **Delivery TTLs** are per kind: stable 10 minutes (replies, inputs), ephemeral
  2 minutes (acks, notices), topic 5 minutes (`ensure-topic`, `close-topic`),
  `delete-conversation` 5 minutes. The manager reads them from
  `CHANNEL_DELIVERY_TTL_{STABLE,EPHEMERAL,TOPIC,DELETE_CONVERSATION}`.

**The receipt-only exception.** The console is the one declared exception, and it is configuration, not code: the adapters in `CHANNEL_RECEIPT_ADAPTERS` (default `console`) acknowledge on receipt, with no delivery TTL and no report.

The manager performs the delivery side effects and deletes the message at the acknowledgement, and the acknowledgement answers `consumed: true`. Anything outside that set gets the generic contract.

The acknowledgement is the console's receipt of the message from the manager, not the push on to browsers, which stays fire-and-forget.

A receipt adapter may read over `GET /channel/ops/stream` instead of polling. The stream pushes each unacknowledged message once per connection as an `op` event and only reads.

After a reconnect it offers again exactly the messages never acknowledged, so there is no cursor and no `Last-Event-ID`. Any other adapter asking for the stream gets `404` and polls.

The console's thread id is assigned by the manager when it enqueues
`ensure-topic` (`console-<conversation uid>`), so no result travels back. It
applies to any receipt adapter by configuration.

**As built.** One bus declaration field, `Policy.DeliveryTTL`, makes a queue a hand-over queue: a claim is the acknowledgement, its lease runs the delivery TTL, and a lapse or a failure is final (dead, never retrying).

New `Bus.Peek` (read, no entry) and `Bus.Acknowledge` (idempotent claim by command id, conditional on the key's last entry, so two replicas hand over once). The `ack` reply field `deliveryTtlSeconds` replaces the old `reclaimAfterSeconds`.

A reconnecting adapter is a fresh GET, so nothing replays.

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
- **The log is a second state home** → declared bounded and lossy. A lost
  log entry never loses a conversation, and the reconciler re-derives owed
  ops.
- **A rate-limited surface still sees waves of retries** → jitter spreads them.
  A per-queue pause is deferred and noted in the open questions.
- **Two retry loops (bus and reconciler)** → the id stays pending while the bus
  retries, so a re-derive is suppressed. After dead, a cool-off prevents an
  instant resubmit.

## Migration Plan

1. Upgrade the chart. A default install on the `nats` state store shares that
   bundle, and one on `kube` renders the dedicated bus bundle. The manager starts
   when the bus is reachable.
2. The log starts empty. Ops owed at the moment of upgrade are re-derived by
   the reconciler, as they are after any restart today.
3. Requests in flight at the upgrade finish under the old manager. New ones
   are accepted by the new one.

4. The wire contract of the channel adapters changed (D-17). A channel adapter
   image is upgraded together with the manager, since one that never
   acknowledges would be offered the same message forever.
5. The tasks, ids and dead buckets an earlier build of this change left on the
   server are inert and may be deleted.

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
