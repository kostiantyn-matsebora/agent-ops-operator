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

The manager SHALL emit one structured activity event per movement it mediates
or is told of.

- Signal receipt, and claim or drop.
- Conversation creation and input queueing.
- Run dispatch and completion, and runtime start.
- Channel op enqueue and completion, and channel inbound.
- Context restore, checkpoint, skip and failure.
- The runtime's model calls and tool calls.

Each event SHALL carry a monotonic `cursor`, an RFC3339 `ts`, a `kind`,
`from` and `to` node references, and a `status` of `ok` or `error`.

Where they apply, it SHALL also carry `conversation`, `pipeline`, `runId`,
`opId`, `inputId`, `latencyMs`, a human-readable `detail` and a bounded
structured `data` map of facts that exist nowhere else.

`from` and `to` SHALL name nodes as the topology graph names them, so an event
is renderable as motion along an existing edge without further inference.

Content that has a durable home (e.g. an input's text, a run's result, an
op's message) SHALL NOT be copied onto the event.

#### Scenario: A conversation's lifecycle emits a complete hop sequence
- **WHEN** a signal is accepted, claimed, a Conversation is created, a runtime pod is started, a run is dispatched with two turns and completed, and a result is sent to a channel
- **THEN** the log holds `signal.received`, `signal.claimed`, `conversation.created`, `runtime.starting`, `run.dispatched`, two `model.call`, `run.completed` and `channel.op.enqueued` events in that order, sharing the conversation, the run events sharing the run's id

#### Scenario: A dropped signal is recorded with its reason
- **WHEN** a signal arrives for a SignalSource no Pipeline claims
- **THEN** a `signal.dropped` event is emitted carrying the source as `from`, no `to`, `status: error`, and the `Wired=False` reason in `detail`

#### Scenario: A runtime pod being created is its own hop
- **WHEN** the manager creates a runtime pod for an admitted conversation
- **THEN** a `runtime.starting` event is emitted with the conversation as `from` and the runtime as `to`, before the pod becomes ready

#### Scenario: Failure is recorded, not omitted
- **WHEN** a run completes with a non-zero exit code, or a channel op is completed with an error
- **THEN** the corresponding event is emitted with `status: error` and the reported reason in `detail`

#### Scenario: The durable record stays where it is
- **WHEN** a run completes with a result
- **THEN** the completion event carries the run's id, exit and duration, and the result is read from the conversation's status

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

### Requirement: The runtime's model calls and tool calls are recorded as hops

A runtime SHALL be able to report, with its work result, a bounded list of the
turns and tool calls the run made.

- For a turn: the model, the tokens in and out, the cache reads and the stop
  reason.
- For a tool call: the tool, the target server where the tool is an MCP tool,
  the duration and the result size.

The list SHALL be bounded in count and in field size, and a runtime that
reports nothing SHALL remain conformant.

The manager SHALL emit one `model.call` hop per turn and one `tool.call` hop
per tool call, attributed to the conversation, pipeline and run, with the
runtime image as `from` and the model or MCP server as `to`. A built-in tool
call SHALL carry no `to`.

**The recorded `from`/`to` name the ENDPOINTS**, never a step the traffic
passes through. Where egress mediation sits between them, the Components
view draws it as a static edge alongside the recorded hop.

That static edge is not a `from` or `to` the event itself carries.

These hops SHALL carry their facts in a bounded structured `data` map. No
prompt text, tool input or tool output SHALL be recorded in telemetry.

#### Scenario: A run's turns are visible on the graph
- **WHEN** a runtime reports three turns and two MCP tool calls with its result
- **THEN** the log holds three `model.call` hops from the runtime image to the model and two `tool.call` hops from the runtime image to the MCP server, sharing the run's id, each with its tokens or its tool name

#### Scenario: A silent runtime draws nothing new
- **WHEN** a runtime reports a result with no turns
- **THEN** the run's dispatch and completion hops are recorded as before and no model or tool hop is emitted

#### Scenario: Content stays out of telemetry
- **WHEN** a tool call's input is a shell command
- **THEN** the hop records the tool's name and duration and not the command

### Requirement: Node kinds cover the components and the externals

Activity events SHALL be able to name a runtime image, a model, an MCP server
and an external system as nodes, so the Components and Infrastructure views
can render every hop as motion along an edge they draw, with no inference in
the browser.

#### Scenario: A model is a node
- **WHEN** a `model.call` hop is emitted
- **THEN** its `to` names the model as the topology names it, and the Components view draws the recorded edge to that model plus the static egress-proxy edge alongside it
