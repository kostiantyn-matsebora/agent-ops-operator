## Purpose

The `Signal` CRD (`agentops.dev/v1alpha1`) and its lifecycle, as a message
broker with the Kubernetes API as its only store:

- PUBLISH — `/signal/inbound` validates, creates one `Signal` per admitted
  signal and acknowledges, and decides nothing else
- CONSUME — on the leader, a pool of concurrent consumers routes each
  `Signal`, one consumer per `Signal` at a time
- HOUSEKEEPING — the leader alone bounds, fails and collects

Publication and consumption share nothing but the CRs, so consumption can
stop while publication keeps storing.

## ADDED Requirements

### Requirement: The Signal records its processing state

The `Signal` SHALL carry every fact publication, consumption and
housekeeping act on. No process SHALL hold a fact about a signal that the
CR does not.

`spec` is written by PUBLICATION only:

| Field | Type | Written | Means |
|---|---|---|---|
| `spec.sourceRef` | `{name}` | at create | the `SignalSource` it arrived on, also its ownerRef |
| `spec.adapter` | string | at create | the serving `SignalAdapter`'s name, a display copy |
| `spec.initiationId` | string | at create | the sender's correlation id — the consumer's key for finding the conversation, never read at publication |
| `spec.kind` | string | at create | `alert` (absent), `job`, `task` or `chat` |
| `spec.title`, `spec.severity`, `spec.labels`, `spec.reader` | as on the normalized signal | at create | attribution and the originating surface, from the arrival |
| `spec.payload` | string | at create | this one message's content |
| `spec.signalId` | string, optional | at create | the sender's per-MESSAGE id, stable across re-sends of that message. When present it fixes the Signal's name — see "A re-send of the same message is deduplicated" |

`metadata.name` is `sig-<hash>` for a signal carrying a `signalId` (the hash
is defined below), and a generated name otherwise.

`spec` is never rewritten after create. Every arriving signal is a unique
message: a `Signal` has one payload and one arrival.

The payload is NOT parsed at publication. An addressed chat target
(`/<pipeline> <task>`) is parsed by the consumer from `spec.payload` and
recorded as the Signal's one claimant. No `spec` field names a target.

`status` is written by the CONSUMER and HOUSEKEEPING only:

| Field | Type | Written by | Means |
|---|---|---|---|
| `status.phase` | enum | the consumer, housekeeping | the roll-up. Absent reads as `Pending` |
| `status.reason` | enum | the consumer, housekeeping | why a `Dropped` signal dropped. Absent for every other phase |
| `status.attempts` | int | the consumer | processing attempts started, monotonic |
| `status.firstAttemptAt` | time | the first attempt | the origin `SIGNAL_RETRY_DEADLINE` measures from |
| `status.completedAt` | time | the write that makes the phase terminal | when the `Signal` went terminal — the origin every TTL measures from |
| `status.consumedBy` | string | the write that makes the phase terminal | the leader's identity, the same string as the leader Lease's `holderIdentity` |
| `status.consumedAt` | time | the same write | when that identity processed it |
| `status.claimants[]` | list | the consumer | the per-claimant workflow, snapshotted once |
| `status.claimants[].ref` | `{kind, name}` | the consumer | the `Pipeline` or `Coordinator` |
| `status.claimants[].state` | `Routing` \| `Routed` \| `Failed` \| `Skipped` | the consumer, housekeeping | that claimant's workflow |
| `status.claimants[].conversationRef` | `{name}` | the consumer | the conversation, once `Routed` |
| `status.claimants[].reason` | string | the consumer, housekeeping | why `Failed` or `Skipped`, verbatim |

A `Signal` that was routed on its second attempt reads, for example:

```yaml
apiVersion: agentops.dev/v1alpha1
kind: Signal
metadata:
  name: sig-3f9a1c0e5b7d4a2e8c61b0d9f4a37e12
  ownerReferences: [{kind: SignalSource, name: node-alerts}]
spec:
  sourceRef: {name: node-alerts}
  adapter: alertmanager
  initiationId: alert-disk-full-node-a
  signalId: "am:9b2c...:fp1"
  kind: alert
  payload: "disk 91% on node-a"
status:
  phase: Routed
  attempts: 2
  firstAttemptAt: "2026-10-27T10:00:00Z"
  completedAt: "2026-10-27T10:02:06Z"
  consumedBy: agentops-manager-5d9f-abcde_6b1e...
  consumedAt: "2026-10-27T10:02:06Z"
  claimants:
  - ref: {kind: Pipeline, name: k8s-ops}
    state: Routed
    conversationRef: {name: alert-disk-full-node-a}
```

#### Scenario: A terminal Signal names its consumer

- **WHEN** the consumer makes a `Signal` terminal
- **THEN** `status.consumedBy` equals the leader Lease's `holderIdentity`,
  `status.consumedAt` and `status.completedAt` are set, and
  `status.attempts` counts the attempts made

#### Scenario: A retried Signal counts its attempts

- **WHEN** a machine signal waits on a full backlog and is routed on its
  second pass
- **THEN** `status.attempts` is 2, and `status.firstAttemptAt` is the first
  pass's time

### Requirement: Publication stores and acknowledges, nothing else

`POST /signal/inbound` SHALL validate the batch, apply the source's
`attributeFilters`, create ONE `Signal` per admitted signal, and then
respond.

- A batch of N admitted signals yields N `Signal`s, even when several share
  an `initiationId`.
- The one exception is a signal whose `signalId` names a `Signal` that
  already exists, below.

Publication SHALL look up no existing `Signal` and make NO routing decision:

- no claim lookup, no `Wired` check, no ambiguity check
- no cooldown consulted or spent
- no capacity check, no addressing parse
- no conversation created or appended, no chat surface told

`attributeFilters` stay SOURCE ADMISSION: a filtered-out signal is refused
at publication, persists nothing, and is reported on the response as it is
today.

Publication SHALL run on the leader. A non-leader's `leaderProxyMiddleware`
forwards the call, unchanged.

Every admitted signal that can open or continue a conversation SHALL be
published this way — machine kinds, an unaddressed chat message, and an
addressed `/<pipeline> <task>`.

A chat COMMAND that only answers — the listing command, a bare
`/<pipeline>` asking for its task, `/close` or `/exit` sent on a general
surface — opens and continues nothing, so it is not a signal: the endpoint
answers it in place, as today, and persists no `Signal`.

The response SHALL be written only after every admitted signal's CR exists.
A signal whose CR cannot be written SHALL NOT be acknowledged. The response
is an error, and the adapter's at-least-once re-send posts the batch again.

- A re-send of a signal carrying a `signalId` is deduplicated at
  publication, below.
- A re-send of a signal without one yields a further `Signal`. The
  consumer makes that safe — by cooldown and by equality reuse.

Each `Signal` SHALL carry an ownerRef to its `SignalSource`.

#### Scenario: The CR exists before the acknowledgement

- **WHEN** an adapter posts an admitted signal to the inbound endpoint
- **THEN** a `Signal` CR owned by the named `SignalSource` exists before
  the `200` response is written

#### Scenario: A signal that cannot be stored is not acknowledged

- **WHEN** writing the `Signal` CR fails
- **THEN** the response is a `5xx` error instead of `200`, and a re-send of
  the batch is accepted: signals with a `signalId` that did land are
  deduplicated, the rest are stored

#### Scenario: Publication routes nothing

- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** the response acknowledges it as queued, a `Signal` exists with
  no `status.phase`, and no conversation, cooldown entry or chat notice
  came from the call

#### Scenario: An addressed chat message is published like any other

- **WHEN** a chat signal whose payload is `/k8s-ops check nodes` is posted
- **THEN** a `Signal` is created with that payload verbatim, and no
  conversation exists until a consumer processes it

#### Scenario: Every admitted signal is its own Signal

- **WHEN** one batch carries three signals, two of them with the same
  `initiationId`
- **THEN** three `Signal`s exist, none absorbed into another

#### Scenario: A filtered signal is refused at publication

- **WHEN** a signal fails one of its source's `attributeFilters`
- **THEN** no `Signal` is created for it, and the response reports the
  filtered drop

### Requirement: A re-send of the same message is deduplicated

A signal MAY carry an optional `signalId`: the sender's id for this one
MESSAGE, unique per message and stable across every re-send of it. It is
not the `initiationId`, which correlates MANY messages on purpose.

- Bounded: 1 to 256 bytes, printable ASCII without spaces. Any other value
  refuses the WHOLE batch with `400` naming `signalId`, before anything is
  stored.
- Opaque to the manager: compared for equality, never parsed.

A signal carrying a `signalId` SHALL be created under the name
`sig-<h>`, where `<h>` is the first 32 hex characters of the SHA-256 of the
source name, a NUL byte and the `signalId`.

- The source is in the hash, so two sources never collide on one id.
- A signal without a `signalId` is created under a generated name, as
  before.

Publication SHALL be idempotent per `(source, signalId)`. Any number of
signals carrying the same pair land as exactly ONE `Signal`:

- in separate requests, or twice within one batch
- whatever phase the existing `Signal` is in, open or terminal, until its
  TTL deletes it
- when two requests race on the same pair, because the deterministic name
  makes the API server's create the single arbiter

Publication stays lookup-free. A create answered `AlreadyExists` IS the
duplicate. It is never an error response and never a second write:

- nothing is stored or changed, and no routing is repeated
- the signal still counts in `queued` and the batch is acknowledged
- the duplicate is recorded as an activity event with code `duplicate` and
  counted, never silent

The window is the life of the `Signal`. A re-send is deduplicated for as
long as the `Signal` exists — through every phase, until the TTL deletes
it. A re-send after the deletion is a new message, and the consumer's
cooldown and equality reuse then apply.

A `signalId` therefore removes re-send churn from the per-source cap, as
the cooldown removes repeats of a correlation id.

#### Scenario: A re-send with the same signalId yields one Signal

- **WHEN** an adapter posts a signal with `signalId` `m-1`, then re-sends
  it
- **THEN** one `Signal` exists, both responses report `queued: 1`, and the
  second records a `duplicate` activity event

#### Scenario: The same signalId twice in one batch yields one Signal

- **WHEN** one batch carries two signals with `signalId` `m-1`
- **THEN** one `Signal` exists, the response is `200` with `queued: 2`, and
  one `duplicate` event is recorded

#### Scenario: A re-send after the Signal was routed changes nothing

- **WHEN** the `Signal` for `signalId` `m-1` is `Routed` and the same
  message is posted again
- **THEN** the response is `200` with `queued: 1`, the `Signal` and its
  conversation are untouched, and no routing happens again

#### Scenario: A concurrent race yields one Signal

- **WHEN** two requests carrying `signalId` `m-1` reach publication at the
  same moment
- **THEN** one create wins, the other is answered `AlreadyExists`, both
  respond `200`, and one `Signal` exists

#### Scenario: The same initiationId with different signalIds is two messages

- **WHEN** two signals share an `initiationId` and carry `signalId`s `m-1`
  and `m-2`
- **THEN** two `Signal`s exist

#### Scenario: No signalId means no deduplication at publication

- **WHEN** the same signal without a `signalId` is posted twice
- **THEN** two `Signal`s exist, and the consumer resolves the repeat by
  cooldown or equality reuse

#### Scenario: Two sources never collide

- **WHEN** two sources each receive a signal with `signalId` `m-1`
- **THEN** two `Signal`s exist, under different names

#### Scenario: Deduplication holds only while the Signal exists

- **WHEN** a `Signal` with `signalId` `m-1` has been deleted by its TTL and
  the same message is posted again
- **THEN** a new `Signal` is created

#### Scenario: An invalid signalId refuses the batch

- **WHEN** a signal carries a `signalId` of 300 bytes
- **THEN** the response is `400` naming `signalId` and nothing is stored

### Requirement: The leader consumes signals with a pool of consumers

The consumer SHALL run on the LEADER only. The manager's channel-operation
queue, cooldown record, activity log and admission gate are per-process
state that only the leader holds, so a consumer elsewhere would decide on
state it does not have.

The consumer is a pool of `SIGNAL_CONSUMER_CONCURRENCY` consumers (chart
`signalConsumerConcurrency`), default **4**, fed by a work queue keyed by
`Signal`:

- exactly one consumer holds a given `Signal` at a time
- a `Signal` still non-terminal after a pass is requeued, with a delay
- there is no claim or lease on the CR: exclusion is the work queue's

The consumer pool and housekeeping SHALL start only on the leader and stop
when it loses the Lease.

Messages sharing a `(source, initiationId)` feed one conversation, so the
consumers SHALL process them ONE AT A TIME in arrival order — by
`creationTimestamp`, then by name. Messages with different keys run
concurrently.

Every pass SHALL be idempotent, so a pass interrupted by a crash or a
leader change is simply repeated.

#### Scenario: Two messages of one conversation are processed in order

- **WHEN** two `Signal`s with the same source and `initiationId` are waiting
  and four consumers are free
- **THEN** the earlier is processed first, the later only after it ends its
  pass, and exactly one conversation exists with the two payloads appended
  in arrival order

#### Scenario: Two different conversations run in parallel

- **WHEN** two `Signal`s with different `initiationId`s are waiting
- **THEN** two consumers process them at the same time

#### Scenario: One consumer per Signal

- **WHEN** a `Signal` is requeued while a consumer is still processing it
- **THEN** no second consumer starts on it until the first pass ends

#### Scenario: The pool starts and stops with the Lease

- **WHEN** a replica gains the leader Lease
- **THEN** its consumer pool and housekeeping start
- **AND** when it loses the Lease they stop

### Requirement: The consumer makes every routing decision

Processing SHALL be the consumer's alone, in this order, reading the
`Signal` it holds:

1. A chat signal addressing a `/<pipeline> <task>` or `/<coordinator>
   <task>` takes that target, looked up by name, as its ONE claimant — no
   claim check, no `Ready` check, no cooldown.
2. Otherwise the claimants are the source's Ready `Pipeline`s and
   `Coordinator`s, read at the first processing attempt. None → `Dropped`,
   reason `unclaimed`. A chat signal with two or more → `Dropped`, reason
   `ambiguous`.
3. Cooldown is consulted and spent, on the `SignalSource` → a suppressed
   signal ends `Dropped`, reason `cooldown`.
4. A chat signal at a full pending backlog → `Dropped`, reason `capacity`.
5. The claimants are snapshotted and each is routed.

A chat signal whose `agentops.dev/channel` names no existing `Channel` ends
`Dropped`, reason `unaddressable` — there is nowhere to answer.

A chat signal ending `Dropped` for `unclaimed`, `ambiguous` or `capacity`
SHALL be told on its originating surface. The consumer enqueues the notice on
the existing in-memory channel-operation queue, as the inbound call did
before. That is fire-and-forget: a lost notice loses no fact, because the
`Signal` records the `Dropped` reason.

Each claimant SHALL be routed idempotently by equality reuse:

- the match is an open conversation the claimant itself originated whose
  `conversationId` equals the signal's `initiationId`
- the lookup SHALL read the API server directly, never an informer cache
- one claimant's conversation never satisfies another's lookup
- a match means the conversation exists, so the consumer appends this
  signal's payload as a recurrence and creates no second one
- a repeat of an `initiationId` whose conversation is open is therefore
  routed by equality reuse, never dropped as a duplicate
- an addressed claimant's conversation adopts the `initiationId` as its
  `conversationId`, the same as every other claimant's

The append SHALL be idempotent per `Signal`. The consumer writes the
`ConversationInput` under a name derived deterministically from the
`Signal`'s name and the claimant, and an input already existing under that
name counts as delivered. A repeated pass therefore appends nothing twice.

A machine signal (`alert`, `job`, `task`) whose claimant cannot open a
conversation for a full backlog SHALL stay `Routing`, and the consumer
requeues it with a delay.

The consumer SHALL move the source's `status.receivedTotal` and
`lastReceived` for each signal it routes, retrying on conflict.

#### Scenario: An unclaimed signal drops at consumption

- **WHEN** a consumer processes a `Signal` whose source no Ready Pipeline
  or Coordinator claims
- **THEN** the pass records `Dropped` with reason `unclaimed`, and no
  conversation is created

#### Scenario: A crash between create and the status write does not duplicate

- **WHEN** a consumer creates a claimant's conversation and the leader stops
  before the status write
- **THEN** the repeated pass's direct lookup finds the open conversation that
  claimant originated whose `conversationId` equals the `initiationId`,
  creates no second conversation, and records `Routed`

#### Scenario: A repeat of an open conversation's id is appended

- **WHEN** a second `Signal` arrives whose `initiationId` equals an open
  conversation's `conversationId` for the claimant, outside the cooldown
- **THEN** its consumer appends the payload to that conversation as a
  recurrence, creates no second conversation, and records `Routed`

#### Scenario: A repeated pass appends once

- **WHEN** a consumer appends a `Signal`'s payload to a conversation and the
  leader stops before the status write, and the new leader repeats the pass
- **THEN** the pass finds the input named for that `Signal` and claimant,
  appends nothing again, and records `Routed`

#### Scenario: One claimant's conversation never answers for another

- **WHEN** two claimants share a signal and only the first has an open
  conversation with the matching `conversationId`
- **THEN** the second claimant's attempt creates its own conversation, and
  the first's is left untouched

#### Scenario: An addressed signal has the target as its one claimant

- **WHEN** a consumer processes a chat `Signal` whose payload is
  `/k8s-ops check nodes` on a source two Pipelines serve
- **THEN** `status.claimants[]` holds `k8s-ops` alone, `Routed` to a
  conversation whose `conversationId` is the `initiationId`

#### Scenario: A full backlog makes a machine signal wait

- **WHEN** a consumer processes an alert `Signal` while the pending backlog
  is full
- **THEN** its claimant stays `Routing`, the consumer requeues it with a
  delay, and a later pass routes it once the backlog drains

#### Scenario: Concurrent consumers keep the cooldown consistent

- **WHEN** two consumers process fresh signals of one source at the same time
- **THEN** both cooldown entries are recorded, neither overwriting the
  other, and a repeated `initiationId` is suppressed exactly once

### Requirement: Routing is tracked per claimant

At the first processing attempt that reaches step 5, the consumer SHALL
snapshot the claimant set into `status.claimants[]`: one entry per
claimant, carrying its ref and a state.

| State | Means |
|---|---|
| `Routing` | the initial, non-terminal state |
| `Routed` | terminal — the entry records the conversation |
| `Failed` | terminal — the entry records the failure reason verbatim |
| `Skipped` | terminal — routing was deliberately not attempted, with the reason recorded |

In this change exactly one condition SHALL produce `Skipped`: the
snapshotted claimant no longer exists when a pass would route it. A
later change may add conditions — a per-binding filter is the anticipated
one.

The snapshot SHALL never grow or shrink once written. A re-wire never
changes an in-flight signal's fan-out.

A later pass SHALL route only non-terminal claimants.

#### Scenario: A re-wire does not reach an in-flight signal

- **WHEN** a Pipeline claims the source after a `Signal`'s claimants were
  snapshotted
- **THEN** the `Signal`'s claimant set is unchanged, and only later
  signals fan out to the new claimant

#### Scenario: A repeated pass leaves terminal claimants alone

- **WHEN** a `Signal` with one `Routed` and one `Routing` claimant is
  processed again
- **THEN** only the `Routing` claimant is attempted, and the `Routed`
  entry and its conversation are untouched

#### Scenario: A deleted claimant is skipped, never failed

- **WHEN** a snapshotted Pipeline is deleted before a non-terminal
  claimant's next pass
- **THEN** that claimant becomes `Skipped` with the reason recorded, and
  the phase rolls up from the remaining entries

### Requirement: The phase is a roll-up derived from the claimants

A `Signal` SHALL report a phase recomputed from `status.claimants[]` on
every status write, except a drop, which precedes any claimant.

| Phase | Means |
|---|---|
| `Pending` (or absent) | not yet consumed, or every snapshotted claimant still `Routing` |
| `Routed` | every claimant terminal, at least one `Routed` |
| `PartiallyRouted` | at least one `Routed`, at least one non-terminal |
| `Failed` | every claimant terminal, none `Routed` |
| `Dropped` | terminal before fan-out, with `status.reason` |

| `status.reason` | Ended by |
|---|---|
| `unclaimed` | no Ready Pipeline or Coordinator serves the source |
| `ambiguous` | an unaddressed chat signal on a source several serve |
| `unaddressable` | a chat signal whose originating `Channel` does not exist |
| `cooldown` | suppressed inside the source's cooldown window |
| `capacity` | a chat signal at a full pending backlog |
| `overflow` | evicted to hold the per-source cap |
| `exhausted` | the retry deadline passed before fan-out |

The write that first makes the phase terminal SHALL set
`status.completedAt`. A later write SHALL never move it.

`Failed`, and every `Dropped` except `cooldown`, SHALL be surfaced as loudly
as the unwired drop is today: an activity event and a log line.

#### Scenario: Full routing rolls up to Routed

- **WHEN** a `Signal`'s last non-terminal claimant becomes `Routed`
- **THEN** the same status write reports phase `Routed` and sets
  `status.completedAt`

#### Scenario: Total failure is loud

- **WHEN** every claimant of a `Signal` is terminal and none is `Routed`
- **THEN** the phase is `Failed`, an activity event is emitted, and the
  failure is logged

#### Scenario: A drop records why

- **WHEN** a `Signal` goes terminal before fan-out
- **THEN** its phase is `Dropped` and `status.reason` is one of the seven
  reasons

### Requirement: Housekeeping is the leader's alone

Only the replica holding the leader Lease SHALL run housekeeping, deletes
included. It starts and stops with the consumer pool.

Housekeeping acts on a `Signal` through the consumer's work queue, so it
never races a consumer's pass. Each of its writes is conditional on the
`resourceVersion` it read.

| Bound | Rule |
|---|---|
| retry deadline | `SIGNAL_RETRY_DEADLINE` (chart `signalRetryDeadline`), default **1h**, from `status.firstAttemptAt`. Past it, every non-terminal claimant becomes `Failed` with the exhaustion as its reason — or, before fan-out, the signal is `Dropped` with reason `exhausted` |
| per-source cap | `MAX_SIGNALS_PER_SOURCE` (chart `maxSignalsPerSource`), default **200** non-terminal `Signal`s. Beyond it, the OLDEST is evicted, counted and logged — `Dropped` with reason `overflow` before fan-out, each non-terminal claimant `Failed` with reason overflow after it |

A terminal `Signal` SHALL be deleted once its phase's TTL has passed,
measured from `status.completedAt` — never from `creationTimestamp`, which
a long-retried signal left behind long ago:

| Phase | TTL | Default |
|---|---|---|
| `Dropped` | `SIGNAL_TTL_DROPPED` (chart `signalTtlDropped`) | **4h** |
| `Routed` | `SIGNAL_TTL_ROUTED` (chart `signalTtlRouted`) | **8h** |
| `Failed` | `SIGNAL_TTL_FAILED` (chart `signalTtlFailed`) | **72h** — the record an operator investigates |

A housekeeping write that makes a `Signal` terminal SHALL record the leader's
identity as `consumedBy`. A poison or permanently failing signal is bounded
by the retry deadline alone.

`Dropped` stays strictly pre-fan-out. Deleting a `SignalSource` SHALL
garbage-collect its `Signal`s through the ownerRef, whatever phase each is
in.

The object count is bounded by these three together: the per-source cap on
live `Signal`s, the consumer-side cooldown that ends a repeat inside its
window as `Dropped` with reason `cooldown`, and the TTLs that delete
terminal ones. No absorb exists to collapse repeats.

A `signalId` removes re-send churn from this count and nothing else.

No `Signal` SHALL sit in a non-terminal phase indefinitely.

#### Scenario: Housekeeping runs on the leader only

- **WHEN** a non-leader replica observes a `Signal` past its TTL
- **THEN** it leaves it, and the leader's housekeeping deletes it

#### Scenario: An exhausted claimant goes terminal

- **WHEN** a `Routing` claimant passes the retry deadline while another
  claimant is already `Routed`
- **THEN** the exhausted claimant becomes `Failed` with the reason
  recorded, and the phase rolls up to `Routed`

#### Scenario: A poison signal ends Failed

- **WHEN** every pass over a `Signal` fails and the retry deadline passes
- **THEN** housekeeping makes every non-terminal claimant `Failed` — or the
  signal `Dropped` with reason `exhausted` before fan-out — loudly

#### Scenario: Overflow evicts the oldest, loudly

- **WHEN** a fresh signal makes its source exceed the cap on non-terminal
  `Signal`s
- **THEN** the oldest non-terminal `Signal` not held by a consumer is evicted, counted
  and logged — `Dropped` with reason `overflow` if it never fanned out

#### Scenario: Overflow after fan-out exhausts claimants, never re-drops

- **WHEN** the eviction candidate is `PartiallyRouted`, one claimant
  `Routed` and one `Routing`
- **THEN** the `Routing` claimant becomes `Failed` with reason overflow and
  the phase rolls up to `Routed`, never to `Dropped`

#### Scenario: Each terminal phase is collected on its own TTL

- **WHEN** a `Dropped`, a `Routed` and a `Failed` `Signal` go terminal at
  the same moment, under the default TTLs
- **THEN** the `Dropped` one is deleted after 4h, the `Routed` one after
  8h, and the `Failed` one after 72h

#### Scenario: The TTL runs from completion, not creation

- **WHEN** a `Signal` created 50 minutes ago goes `Routed` now
- **THEN** it is deleted 8h after its `status.completedAt`, not 8h after
  its creation

#### Scenario: A deleted source takes its signals

- **WHEN** a `SignalSource` is deleted while `Signal` CRs of every phase
  reference it
- **THEN** all of them are garbage-collected through the ownerRef

### Requirement: Every signal bound is a chart value

Every bound this capability names SHALL be a value of the manager's chart,
rendered into the manager Deployment's environment. None is a constant in
code.

| Setting | Chart key | Manager env var | Default |
|---|---|---|---|
| Routed TTL | `signalTtlRouted` | `SIGNAL_TTL_ROUTED` | `8h` |
| Dropped TTL | `signalTtlDropped` | `SIGNAL_TTL_DROPPED` | `4h` |
| Failed TTL | `signalTtlFailed` | `SIGNAL_TTL_FAILED` | `72h` |
| retry deadline | `signalRetryDeadline` | `SIGNAL_RETRY_DEADLINE` | `1h` |
| live signals per source | `maxSignalsPerSource` | `MAX_SIGNALS_PER_SOURCE` | `200` |
| consumer consumers | `signalConsumerConcurrency` | `SIGNAL_CONSUMER_CONCURRENCY` | `4` |

- The keys sit beside `signalTtlDropped`, `signalTtlRouted`,
  `signalRetryDeadline` and `maxSignalsPerSource` in the chart's values.
- A duration is a Go duration such as `90m`. An unparseable or non-positive
  one follows the manager's convention for its other duration settings: the
  default applies, and the manager logs a warning naming the variable.
- `docs/configuration.md` SHALL list each key with its meaning and default.

#### Scenario: A chart override reaches the manager

- **WHEN** a release sets `signalTtlRouted: 6h`
- **THEN** the rendered manager Deployment carries `SIGNAL_TTL_ROUTED=6h`,
  and a `Routed` `Signal` is deleted 6h after its `status.completedAt`

#### Scenario: The default applies when unset

- **WHEN** a release sets none of the keys
- **THEN** the manager runs with the defaults in the table

#### Scenario: A bad duration falls back loudly

- **WHEN** `SIGNAL_TTL_ROUTED` is `soon`
- **THEN** the manager uses `8h` and logs a warning naming
  `SIGNAL_TTL_ROUTED`

### Requirement: A leader change resumes every non-terminal Signal

All signal state SHALL be on `Signal` CRs. A new leader SHALL start the
consumer pool and housekeeping with the Lease, and the pool SHALL reconcile
every non-terminal `Signal` from the CRs.

No routing decision SHALL come from process memory that was not just read
from a CR. A pass interrupted by the old leader's stop is repeated, and its
idempotency — equality reuse and the input named for the `Signal` — keeps it
from duplicating an effect.

#### Scenario: A leader change loses no accepted signal

- **WHEN** the leader stops while some `Signal`s are being processed and
  others wait
- **THEN** the new leader processes them all from the CRs, and none is lost
  or routed twice

#### Scenario: Memory is never the record

- **WHEN** leadership moves to a replica that shared no memory with the old
  leader
- **THEN** every decision it makes about a `Signal` is derived from CRs it
  reads

### Requirement: Consumption is separable from publication

Publication SHALL NOT depend on the consumer running. A `Signal` taken by no
consumer SHALL wait, non-terminal, and SHALL be processed as soon as the pool
runs.

This is the seam a later change gates — pausing consumption is the pool not
taking `Signal`s, while publication keeps storing.

#### Scenario: Signals wait while the consumer is stopped

- **WHEN** a manager runs with its consumer pool stopped and an adapter
  posts three signals
- **THEN** all three are acknowledged and stored as `Signal`s with no
  phase, and starting the pool routes them

### Requirement: Only the manager writes a Signal

No component other than the manager SHALL write a `Signal` — create,
update, status or delete. Every other component, the console included,
SHALL hold read-only access at most.

#### Scenario: The console reads without writing

- **WHEN** the console renders signals and their per-claimant state
- **THEN** it does so through watch and read access alone, holding no
  write verb on the kind

#### Scenario: A non-manager write is refused

- **WHEN** any component other than the manager attempts to write a
  `Signal`
- **THEN** the cluster's authorization refuses it, because no grant beyond
  the manager's exists

### Requirement: Cooldown suppression is recorded, never silent

Cooldown SHALL be consulted by the consumer, and SHALL keep its key and its
persistence on the `SignalSource` — this change moves neither. The check and
the spend SHALL be one atomic step per source, so concurrent consumers never
both pass the same `initiationId`.

A cooldown-suppressed signal SHALL become `Dropped` with reason `cooldown`,
rather than vanishing without a record.

#### Scenario: A suppressed signal leaves a record

- **WHEN** a consumer processes a `Signal` whose `initiationId` was
  admitted on the same source inside its cooldown window
- **THEN** no conversation or input is created, and the `Signal` reports
  `Dropped` with reason `cooldown`

## Future

Not a requirement. A consumer on a non-leader replica would need the
cooldown record and the channel-operation queue made safe across replicas,
or reached through calls bound to the leader. That is its own change.
