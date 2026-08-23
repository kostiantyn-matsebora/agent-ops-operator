# activity-telemetry Specification

## Purpose

The manager's record of every HOP it mediates — signal admitted, conversation
opened, work dispatched, answer delivered — served for replay and as a stream.

It is the declared-LOSSY class of state, deliberately: a bounded in-memory ring
buffer, never persisted, never blocking the operation it records. That is why two
rules matter more than the rest — emission is never itself a signal, and a gap is
REPORTED rather than rendered as silence, because a cursor answered with an empty
list reads as "nothing happened".

The one fact that is NOT telemetry is the latest context checkpoint, which is
durable state on the Conversation: whether a conversation can continue after a
crash cannot depend on a buffer entry that may have been evicted.

## Requirements

### Requirement: The manager records every hop it mediates
The manager SHALL emit one structured activity event per movement it mediates: signal receipt and claim/drop, conversation creation, input queueing, run dispatch and completion, channel op enqueue and completion, and channel inbound. Each event SHALL carry a monotonic `cursor`, an RFC3339 `ts`, a `kind`, `from` and `to` node references (`{kind, name}`), a `status` of `ok` or `error`, and — where they apply — `conversation`, `pipeline`, `runId`, `opId`, `inputId`, `latencyMs` and a human-readable `detail`.

`from` and `to` SHALL name nodes as the topology graph names them, so an event is renderable as motion along an existing edge without further inference.

#### Scenario: A conversation's lifecycle emits a complete hop sequence
- **WHEN** a signal is accepted, claimed by a Pipeline, a Conversation is created, a run is dispatched and completed, and a result is sent to a channel
- **THEN** the activity log contains `signal.received`, `signal.claimed`, `conversation.created`, `run.dispatched`, `run.completed` and `channel.op.enqueued` events, in that order, sharing the same `conversation`, and the run events sharing the same `runId`

#### Scenario: A dropped signal is recorded with its reason
- **WHEN** a signal arrives for a SignalSource no Pipeline claims
- **THEN** a `signal.dropped` event is emitted carrying the source as `from`, no `to`, `status: error`, and the `Wired=False` reason in `detail`

#### Scenario: Failure is recorded, not omitted
- **WHEN** a run completes with a non-zero exit code, or a channel op is completed with an error
- **THEN** the corresponding event is emitted with `status: error` and the reported reason in `detail`

### Requirement: The activity log is bounded, in-memory and lossy by design
The activity log SHALL be a fixed-size in-memory ring buffer, evicting oldest-first, never persisted and never written to any Kubernetes object. Emission SHALL NOT block the operation being recorded: if the buffer is full the oldest event is dropped, and no dispatch, reconcile or HTTP handler waits on it.

The durable record of what happened SHALL remain `Conversation.status.runs[]`.

#### Scenario: A storm evicts rather than grows
- **WHEN** more events are emitted than the buffer holds
- **THEN** the oldest events are dropped, memory stays bounded, and every emitting call path returns without additional latency

#### Scenario: Nothing reaches etcd
- **WHEN** any number of activity events are emitted
- **THEN** no Kubernetes object is created or updated as a result

### Requirement: Activity is served over replay and stream endpoints
The manager SHALL serve `GET /activity?since=<cursor>&limit=<n>` returning events after a cursor, and `GET /activity/stream` as SSE delivering events as they occur, each carrying its cursor. Both SHALL be authenticated with the adapter bearer scheme. A client reconnecting with a cursor older than the buffer's oldest SHALL receive an explicit resync signal rather than a silent gap.

#### Scenario: Reconnect after a gap is explicit
- **WHEN** a client reconnects with a cursor that has been evicted
- **THEN** the response tells it to resync, and the client is expected to re-read snapshots rather than assume continuity

#### Scenario: Unauthenticated access is refused
- **WHEN** either endpoint is called without a valid adapter token
- **THEN** the request is rejected with 401 and no events are disclosed

### Requirement: Adapters may report their own delivery hops
Adapters SHALL be able to `POST /activity` to report hops only they observe — notably `channel.op.completed` with real delivery latency — authenticated with the per-adapter derived token they already hold. An adapter SHALL only be able to report events attributed to itself. Reporting is OPTIONAL: an adapter that reports nothing still appears in the graph via manager-side events.

#### Scenario: Delivery confirmation upgrades an edge
- **WHEN** an adapter reports `channel.op.completed` for an op the manager enqueued
- **THEN** that edge reflects confirmed delivery, and until such a report arrives it reflects "sent, unconfirmed" rather than success

#### Scenario: An adapter cannot report as another
- **WHEN** an adapter posts an event attributed to a different adapter
- **THEN** the request is rejected

### Requirement: Activity emission is never a signal
Activity events SHALL NOT be routed through `/signal/inbound`, and no component SHALL convert an activity event into a signal. agent-ops' own machinery reports STATUS, never SIGNAL.

#### Scenario: Telemetry creates no work
- **WHEN** a large volume of activity events is emitted, including error events about agent-ops' own components
- **THEN** zero Conversations are created as a result

### Requirement: Context operations are recorded as activity

Restoring, checkpointing, skipping and failing a context synchronisation SHALL
each be emitted as an activity event against the conversation they belong to,
carrying at least the duration and the volume of data transferred.

They SHALL be emitted through the existing activity log rather than through a
separate telemetry path, so that the console's per-conversation view and the
metrics registry continue to derive from ONE instrumentation pass. Adding a
second path would allow a metric and its event to disagree, which is precisely
what the single-observer arrangement exists to prevent.

A skipped checkpoint SHALL be emitted, because "nothing changed" and "nothing
ran" are different facts and an operator diagnosing a stale context needs to
tell them apart.

The synchronising process runs in the runtime pod rather than in the manager, so
it SHALL report its operations to the manager, which emits them.

#### Scenario: A checkpoint is visible per conversation

- **WHEN** a conversation's context is checkpointed
- **THEN** the event appears in that conversation's activity, with its duration and size, and the corresponding metric is observed

#### Scenario: A skip is distinguishable from silence

- **WHEN** a periodic checkpoint is skipped because nothing changed
- **THEN** an event records the skip, rather than the interval passing with no trace

#### Scenario: A failure is recorded

- **WHEN** a checkpoint or restore fails
- **THEN** the failure is recorded as activity against the conversation with its reason

### Requirement: The latest checkpoint is durable state, not telemetry

The fact of a conversation's most recent successful checkpoint — when it
happened, which copy it produced, and whether it was taken at a work boundary —
SHALL be recorded on the conversation itself, not only in the activity log.

The activity log is bounded and lossy by design. Whether a conversation has a
usable durable context decides whether continuity is possible after a crash, so
it cannot depend on a record that may have been evicted.

This SHALL be written ONLY when a checkpoint actually transferred data. A
skipped checkpoint SHALL NOT write it. Recording every skip would patch every
conversation on every interval indefinitely, which is the write amplification
that suppressed signals are already required to avoid.

#### Scenario: The conversation knows its own context is safe

- **WHEN** a conversation's context is checkpointed
- **THEN** the conversation records the time, the copy and whether it was quiesced

#### Scenario: A skip costs no write

- **WHEN** a periodic checkpoint is skipped because nothing changed
- **THEN** the conversation is not patched
