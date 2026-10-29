# signal-persistence — design

## Context

See `proposal.md` — Why. Current state this design builds on:

- `/signal/inbound` routes synchronously at ingest
  (`internal/httpapi/signals.go`): source lookup, Wired check, cooldown,
  claim fan-out, equality routing into Conversations.
- A chat message addressed `/<pipeline> <task>` is answered before that
  routing, by the chat router, and opens its conversation on its own.
- Cooldown already persists on the `SignalSource` and rebuilds after a
  restart. Grouping is gone — the sender mints `initiationId` (#387).
- Payloads already land in etcd as `ConversationInput` once routed. Only the
  pre-routing stretch has no durable record.
- Every API call reaches the leader: a non-leader's `leaderProxyMiddleware`
  reverse-proxies it. The leader alone does the work. The cooldown map, the
  channel-operation queue (`OpQueue`) and the activity log are per-process and
  filled only by the leader, which is why the chart's second replica is a
  standby (`durable-chat-ops-broker`, `activity-telemetry`).
- The merged `introduce-control-plane` change states the RBAC pattern for
  manager-owned kinds: the manager holds full CRUD, every other component
  reads at most.

## Goals / Non-Goals

**Goals:**

- Every accepted signal is a CR before it is acknowledged, and nothing
  decides its fate until a consumer reads that CR.
- Publication and consumption are separate. A later change can stop the
  consumer without touching the endpoint.
- A leader that changes loses nothing and duplicates nothing. All state is on
  the CRs, and the new leader resumes every unfinished `Signal`.
- Consumption is concurrent on the leader: several consumers, not one thread.
- Routing outcomes are the existing ones: the same cooldown, fan-out and
  equality reuse.

**Non-Goals:**

- No pause, hold, drop mode, maintenance window or control-plane resident.
  The seam is left, the gate is not built.
- No per-binding filters, no change to claim semantics.
- No message-broker component, in-cluster or external.

## Decisions

### D-1: The queue is the Kubernetes API — a CRD, never a broker component

Held and in-flight signals persist as `Signal` CRs. The invariant rules
everything else out: manager state is Kubernetes API or derivable, the
manager mounts nothing, and an accepted signal is neither derivable nor
declared-lossy, because the adapter will not resend.

Volumes are post-dedup ops signals, tens per hour. The per-source cap and the
TTLs keep the object count bounded (D-11).

**Alternative considered:** an external broker (NATS, Redis Streams).
Rejected. It adds a stateful component, a trust boundary and a credential
custody problem, for throughput this domain never produces. The seam
remains: the store sits behind publish and consume, swappable if volumes
ever change the math.

### D-2: Publish stores, a separate consumer routes

`/signal/inbound` validates, applies `attributeFilters`, creates the `Signal`
and acknowledges. It looks up no claimant, spends no cooldown, checks no
capacity and parses no addressing. Routing is the consumer's alone.

This is a pure publish/subscribe split. Pausing consumption later means the
consumer stops claiming. The endpoint keeps storing, and the signals wait.

The adapter response narrows with it (D-9). The earlier design, which routed
the first attempt inline to keep the response unchanged, is rejected: it left
two routers acting on one `Signal` and made a pause impossible without also
stopping ingest.

A chat command that only answers (`/pipelines`, a bare `/<pipeline>`,
`/close`, `/exit` on a general surface) opens and continues nothing. It is
not a message to queue, so the endpoint answers it in place as today and
stores no `Signal`.

### D-3: A concurrent consumer on the leader

The consumer side is a pool of consumers on the leader, competing on one
process. `SIGNAL_CONSUMER_CONCURRENCY` (chart `signalConsumerConcurrency`,
default 4) sets how many run.

- **Exclusion per `Signal`:** the controller work queue hands one `Signal` to
  one consumer at a time. No claim is written to the CR.
- **Order per conversation:** messages sharing a `(source, initiationId)` feed
  one conversation, so they are processed one at a time in arrival order
  (`creationTimestamp`, then name). Different keys run in parallel. Without
  this, two consumers could append two messages of one chat out of order, or each
  fail to find the conversation the other is creating.
- **Shared state:** the cooldown map is checked and spent atomically per
  source, since consumers share it.
- **Ack:** write the claimants, the phase, `consumedBy` (the leader pod's
  identity, the one the leader Lease holds), `consumedAt` and `attempts`.
- **Wait:** a machine signal at a full backlog stays `Routing`, and the consumer
  requeues it after the reconciler's existing retry interval. It stays
  `Pending` or `PartiallyRouted` while `status.attempts` grows.
- **Per-key lock:** the work queue only guarantees one consumer per object, so
  the pool holds a lock per `(source, initiationId)` and releases it in
  arrival order. Requeued signals take the same lock.

A leader change needs no recovery step. The new leader's consumer reconciles
every non-terminal `Signal`, and idempotency (D-5) makes a redo safe. A
poison message ends at the retry deadline (D-11), never in a loop.

The pool and housekeeping start only on the leader. Pausing consumption later
means the pool stops taking `Signal`s while publish keeps storing.

**Alternative considered:** a CR claim with a lease so that every replica
consumes. The cooldown map, `OpQueue` and activity log are per-process and
leader-filled, so a consumer on a standby would run on state nobody reads.

A standby would need cross-replica safe state, or a call to the leader for
each in-memory step. At tens of signals an hour that buys no throughput. It
is its own change, and this design does not block it.

### D-4: The consumer makes every routing decision

In order, on the `Signal` it claimed:

1. An addressed chat signal (`/<pipeline> <task>` or `/<coordinator>
   <task>`) takes the named target as its one claimant, by name, with no
   claim check, no `Ready` check and no cooldown.
2. Otherwise the claimants are the source's Ready Pipelines and
   Coordinators. None is `Dropped` (reason `unclaimed`). A chat signal with
   several is `Dropped` (reason `ambiguous`).
3. Cooldown is consulted and spent, atomically per source. A suppressed
   signal is `Dropped` (reason `cooldown`).
4. A chat signal at a full pending backlog is `Dropped` (reason `capacity`).
   A machine signal stays `Routing` and is requeued.
5. The claimants are snapshotted and each is routed.

Addressing is parsed here, from the payload. The `Signal` needs no `target`
field, and publish never reads the text.

A chat signal that ends `Dropped` for `unclaimed`, `ambiguous` or `capacity`
has the chat surface told (D-10).

### D-5: Routing is idempotent

A redo after a crash or a leader change must not duplicate a conversation or an input.

- **Conversation create or reuse:** equality reuse, scoped per claimant. The
  match is an open conversation the claimant originated whose
  `conversationId` equals the `initiationId`. The lookup reads the API server
  directly, never the informer cache, so a crash between create and ack finds
  the conversation it just made.
- **Input append:** the consumer writes the `ConversationInput` under a name
  derived from the `Signal`'s name and the claimant. An input that already
  exists under that name counts as delivered. No field and no
  conversation-side bookkeeping is added.
- **Addressed conversations** adopt the `initiationId` as their
  `conversationId`, the same as every other claimant's. They did not before.

### D-6: Claimants are the state machines, phase is a roll-up

`status.claimants[]` carries one workflow per claimant:
`Routing → Routed | Failed | Skipped`. The phase is recomputed from the claimant
states on every status write, never authored independently.

- The claimant set snapshots ONCE, at first processing, the same principle
  that pins a Conversation's refs. A re-wire never changes an in-flight
  signal's fan-out.
- A retry touches only non-terminal claimants. The record is the idempotency
  guard.
- A retry deadline (`SIGNAL_RETRY_DEADLINE`, default 1h) flips exhausted
  claimants to `Failed`, so `PartiallyRouted` always exits.

**Alternative considered:** a single phase field written by the route loop.
Rejected. Partial routing becomes invisible, and a retry cannot know which
claimant already took the signal.

### D-7: One Signal per message, no absorb

Every admitted signal is its own `Signal`. A repeat of an `initiationId` is
not folded into an earlier one.

A `Conversation` already is the accumulator for an `initiationId`. Its id is
that value, and later messages append to it as inputs.

A `Signal` that also accumulated repeats would be a second, worse copy. From
the queue's view every message is unique. Matching it to a conversation is
the consumer's job.

Without absorb, the object count rests on the per-source cap, the consumer's
cooldown and the TTLs. A burst of identical alerts collapses only through
cooldown.

### D-8: `signalId` makes publish idempotent

An adapter may send a per-message `signalId`, stable across a re-send of that
message. It differs from `initiationId`, the correlation id, which is shared
across messages on purpose: a thread's replies and a job's ticks all reuse
it.

The `Signal`'s name derives from `(source, signalId)`:
`sig-<first 32 hex of sha256(source NUL signalId)>`. A create that finds the
name taken is the duplicate. Nothing is written, the response still
acknowledges it, and an activity event with code `duplicate` records it.
Publish therefore needs no lookup and no list.

- It holds across requests, within one batch, and under a concurrent race.
- It holds in any phase until the TTL deletes the `Signal`. After that the
  consumer's cooldown and equality reuse are the backstop.
- A signal with no `signalId` gets a generated name and no dedup. Two
  messages are then two `Signal`s.

Every shipped adapter sends one, with a stability rule per adapter (see
tasks).

Alertmanager and Pulse carry no per-delivery id, so their rule is a body
hash. A re-notification with an identical body is then deduplicated for the
`Signal`'s life, which is stronger than today's cooldown. Tasks record the
decision.

### D-9: The inbound response acknowledges storage

`200 {"queued": n}` where `n` counts signals stored or deduplicated. The
`conversations` field and the routing `reason` are withdrawn. The reason
remains only for an `attributeFilters` refusal, which is decided before
anything is stored. A failed store returns `5xx`, and the adapter's ordinary
at-least-once re-send repeats it.

Adapter contract version **2.3.0**, MINOR. By the page's own rule MAJOR moves
only when an adapter conformant with the previous version stops conforming.
No shipped adapter reads the withdrawn fields, and `signalId` is additive.

An adapter can no longer learn from the response that its source is unwired.
The `Signal` records it (`Dropped`, reason `unclaimed`) and the console shows
it.

### D-10: Chat notices stay fire-and-forget

The consumer, on the leader, enqueues the chat notice (unwired, ambiguous, at
capacity) into the channel-operation queue exactly as the ingest path does
today. That queue is in-memory by design and its notices are declared
fire-and-forget: losing one costs nothing.

The fact is not lost. The `Signal` records `Dropped` and its reason, and the
console shows it.

### D-11: Housekeeping is the leader's alone

The leader, and only the leader:

- flips exhausted claimants to `Failed` at the retry deadline,
- enforces the per-source cap, evicting the oldest non-terminal `Signal`
  (`Dropped` reason overflow before fan-out, the non-terminal claimants
  `Failed` after it),
- deletes finished `Signal`s by TTL, measured from `status.completedAt`.

TTLs are per phase: `SIGNAL_TTL_ROUTED` 8h, `SIGNAL_TTL_DROPPED` 4h,
`SIGNAL_TTL_FAILED` 72h. `ownerRef` to the `SignalSource` takes a deleted
source's signals from any phase.

Housekeeping goes through the same work queue as the consumer, so a retry
deadline or an eviction never collides with a consumer mid-pass. Its writes are
conditional on the `resourceVersion` as a second guard.

`firstAttemptAt` is set on the first pass. A `Signal` that never gets a pass,
because the pool is stopped, is aged only by the per-source cap.

Every bound is a chart value: `signalTtlRouted`, `signalTtlDropped`,
`signalTtlFailed`, `signalRetryDeadline`, `maxSignalsPerSource`,
`signalConsumerConcurrency`.

### D-12: RBAC per the control-plane pattern

The manager's Role gains full CRUD on `signals` (lowercase plural — the
informer breaks silently otherwise). The console's Role gains
`get`/`list`/`watch` only. No other component touches the kind.

### D-13: Backlog-full machine signals wait, chat keeps the decline

With a durable record, dropping an alert because 50 conversations are
`Pending` is no longer forced.

- Machine kinds (alert, job, task) stay `Pending` and route when the backlog
  drains.
- Chat is declined at once and told on the originating surface, addressed
  chat included. A person is answered now, never left waiting silently.

## Risks / Trade-offs

- **A publish failure is unacknowledged.** The response reports `5xx`, and
  safety rests on the adapter contract's at-least-once posture. With a
  `signalId` the re-send is deduplicated, without one it is a second
  `Signal`.
- **No absorb, so a flapping source creates many `Signal`s.** Cooldown drops
  repeats in the window, the cap bounds the worst case and the TTLs delete
  them. The cap now evicts the oldest non-terminal `Signal`, so a long flap
  can churn it. The implementer confirms 200 is still sensible.
- **A body-hash `signalId` deduplicates a re-notification.** Alertmanager and
  Pulse repeat an identical body every `repeat_interval`, and dedup would drop
  that recurrence while the earlier `Signal` exists. Accepted: a changed
  alert carries a new `startsAt` or `endsAt`, so its hash differs, and a
  duplicate conversation input is the worse failure. Decided by the user.
- **Overflow can evict a `Signal` with a `Routing` claimant mid-retry.**
  Accepted, deliberately. The cap protects etcd, and the eviction is counted
  and logged.
- **Concurrent consumers share state.** They share the cooldown map, so its
  check-and-spend is atomic per source. Two messages of one conversation are
  serialized in arrival order.
- **One consumer process.** Throughput is the leader's. At tens of signals an
  hour that is ample, and the consumer count is a chart value.
- **The adapter response narrows.** A caller reading `conversations` loses it.
  No shipped adapter does, and the contract bump records it.
- **A signal squatting non-terminal forever.** The retry deadline
  terminates it, and the TTL removes it.
- **Console screenshot and demo drift.** The UI change re-runs the
  `screenshots` and `demo` npm scripts, per `documentation.md`.

## Migration Plan

Additive. New CRD only.

1. `chart/crds/` gains the `Signal` CRD (controller-gen output). Upgrades run
   `kubectl apply -f chart/crds/` — the standing rule, Helm never upgrades an
   existing CRD.
2. RBAC rows and the six chart values land in the same release as the
   manager that needs them.
3. Rollback: the previous manager ignores the kind entirely. Orphaned
   `Signal` objects are inert and GC with their sources.
4. Adapters ship a `signalId` and the manager ignores it until upgraded, so
   adapter and manager upgrade in either order.

## Open Questions

None.
