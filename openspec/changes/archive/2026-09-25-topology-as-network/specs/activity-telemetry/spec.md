## ADDED Requirements

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
- **THEN** its `to` names the model as the topology names it, and the Components view moves the edge from egress-proxy to that model

## MODIFIED Requirements

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
`from` and `to` node references, a `status` of `ok` or `error`, and, where
they apply, `conversation`, `pipeline`, `runId`, `opId`, `inputId`,
`latencyMs`, a human-readable `detail` and a bounded structured `data` map
of facts that exist nowhere else.

`from` and `to` SHALL name nodes as the topology graph names them, so an event
is renderable as motion along an existing edge without further inference.

Content that has a durable home, an input's text, a run's result, an op's
message, SHALL NOT be copied onto the event.

#### Scenario: A conversation's lifecycle emits a complete hop sequence
- **WHEN** a signal is accepted, claimed, a Conversation is created, a run is dispatched with two turns and completed, and a result is sent to a channel
- **THEN** the log holds `signal.received`, `signal.claimed`, `conversation.created`, `run.dispatched`, two `model.call`, `run.completed` and `channel.op.enqueued` events in that order, sharing the conversation, the run events sharing the run's id

#### Scenario: A dropped signal is recorded with its reason
- **WHEN** a signal arrives for a SignalSource no Pipeline claims
- **THEN** a `signal.dropped` event is emitted carrying the source as `from`, no `to`, `status: error`, and the `Wired=False` reason in `detail`

#### Scenario: Failure is recorded, not omitted
- **WHEN** a run completes with a non-zero exit code, or a channel op is completed with an error
- **THEN** the corresponding event is emitted with `status: error` and the reported reason in `detail`

#### Scenario: The durable record stays where it is
- **WHEN** a run completes with a result
- **THEN** the completion event carries the run's id, exit and duration, and the result is read from the conversation's status
