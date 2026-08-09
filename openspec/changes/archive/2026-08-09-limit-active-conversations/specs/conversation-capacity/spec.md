## ADDED Requirements

### Requirement: Simultaneously active conversations are capped
A conversation SHALL be considered **active** while a runtime pod exists for it
(pod phase `Running` or `Pending`); a conversation whose pod has exited is not
active and SHALL NOT consume capacity. The manager SHALL admit at most
`MAX_ACTIVE_CONVERSATIONS` active conversations at a time (chart
`maxActiveConversations`), default **5**. The limit SHALL be counted from live
runtime pods rather than from conversation status, so a pod that is stuck or a
status patch that was lost cannot inflate available capacity. When
`MAX_ACTIVE_CONVERSATIONS` is unset and the deprecated `MAX_RUNTIMES` is set,
the manager SHALL honor the deprecated value and log that it is deprecated.

#### Scenario: Sixth conversation is not admitted
- **WHEN** five conversations each hold a runtime pod and a signal opens a sixth
- **THEN** no sixth runtime pod is created and the sixth conversation reports phase `Pending`

#### Scenario: Idle conversation frees its slot
- **WHEN** an active conversation's runtime pod exits after its idle TTL
- **THEN** the conversation reports phase `Idle` and no longer counts against the cap

#### Scenario: Deprecated alias is honored
- **WHEN** the manager starts with `MAX_RUNTIMES=8` and no `MAX_ACTIVE_CONVERSATIONS`
- **THEN** the effective cap is 8 and a deprecation notice is logged

### Requirement: Over-cap conversations wait in a Pending phase
`ConversationPhase` SHALL include `Pending`, meaning the conversation exists and
holds its inputs and wiring snapshot but has not been admitted. While a
conversation is `Pending` the manager SHALL NOT create a runtime pod, SHALL NOT
enqueue any `ensure-topic` operation, and SHALL NOT compile or create the
conversation's MCP ConfigMap. Its inputs, `signature` label, and pipeline
wiring snapshot SHALL be retained unchanged so signature grouping and window
reuse treat it exactly like an admitted conversation. `Queued` SHALL keep its
existing meaning — admitted, with work waiting behind the serial-per-conversation
rule — and SHALL NOT be used for capacity waiting.

#### Scenario: Pending conversation provisions nothing
- **WHEN** a conversation is created while the cap is full of busy conversations
- **THEN** it has no runtime pod, no chat topic, and no `agentops-mcp-conv-<name>` ConfigMap

#### Scenario: Grouping still reuses a pending conversation
- **WHEN** a second signal arrives with the same signature as a `Pending` conversation inside the grouping window
- **THEN** the signal is appended as an input to that pending conversation rather than opening another one

#### Scenario: Backlog survives a manager restart
- **WHEN** the manager restarts with pending conversations waiting
- **THEN** those conversations are still pending after restart and are still admitted in their original order

### Requirement: Admission is FIFO by creation time
A `Pending` conversation SHALL be admitted only when a capacity slot is free and
no older `Pending` conversation needing a runtime pod exists. Admission SHALL be
triggered by runtime pod deletion events in addition to periodic reconciliation,
so a freed slot is filled without waiting for a poll interval. On admission the
conversation SHALL proceed through the normal path: topic creation, MCP
resolution, runtime pod creation, and dispatch.

#### Scenario: Oldest pending conversation goes first
- **WHEN** conversations A (older) and B (newer) are both `Pending` and one slot frees
- **THEN** A is admitted and B remains `Pending`

#### Scenario: Freed slot is filled promptly
- **WHEN** an active conversation's runtime pod is deleted and a pending conversation is waiting
- **THEN** the waiting conversation is admitted without waiting for the fallback requeue interval

#### Scenario: Admitted conversation gets its topic
- **WHEN** a pending conversation is admitted
- **THEN** an `ensure-topic` operation is enqueued for each bound channel and a runtime pod is created

### Requirement: Waiting is visible on the originating chat surface
Because a `Pending` conversation has no thread, the manager SHALL post one
message to the general surface of the conversation's originating channel when
the conversation enters `Pending`, stating that it is queued for capacity. The
message SHALL be emitted once per entry into `Pending`, not on every
reconciliation.

#### Scenario: User is told their request is queued
- **WHEN** a person addresses a pipeline from chat while the cap is full
- **THEN** a message on that channel's general surface reports the request is queued for capacity

#### Scenario: No repeated notices
- **WHEN** a pending conversation is reconciled repeatedly before a slot frees
- **THEN** exactly one queued notice has been sent

### Requirement: The pending backlog is bounded
The manager SHALL cap pending conversations at `MAX_QUEUED_CONVERSATIONS`
(chart `maxQueuedConversations`), default **50**. When the backlog is full,
`/signal/inbound` SHALL NOT create a conversation and SHALL report the batch as
dropped with a capacity reason through the existing drop-reason path: a chat
origin SHALL be told on the surface the message came from, and alert or job
origins SHALL be logged and counted.

#### Scenario: Signal declined when the backlog is full
- **WHEN** 50 conversations are `Pending` and another alert signal arrives
- **THEN** no conversation is created and the response reports the batch dropped with a capacity reason

#### Scenario: Chat sender learns the request was declined
- **WHEN** the backlog is full and a person sends a message on a chat surface
- **THEN** a message on that surface explains the system is at capacity

### Requirement: Idle runtime pods release capacity within a minute by default
The manager-wide runtime idle TTL (`RUNTIME_IDLE_TTL_M`, chart
`runtimeIdleTtlMinutes`) SHALL default to **1 minute**.
`AgentRuntime.spec.idleTTLMinutes` SHALL continue to override it per runtime. An
idle runtime pod whose conversation has nothing inflight and nothing queued MAY
still be evicted before its TTL to admit waiting work; eviction SHALL delete only
the pod, leaving the conversation and its session intact so it resumes on its
next input.

#### Scenario: Default idle TTL reaches the pod
- **WHEN** a runtime pod is created with no `AgentRuntime` override and no configured TTL
- **THEN** its `RUNTIME_IDLE_TTL_M` environment variable is `1`

#### Scenario: Per-runtime override still wins
- **WHEN** an `AgentRuntime` sets `idleTTLMinutes: 15`
- **THEN** pods for conversations using that runtime receive `RUNTIME_IDLE_TTL_M=15`

#### Scenario: Eviction does not end the conversation
- **WHEN** an idle conversation's pod is evicted to admit waiting work and a new input arrives afterwards
- **THEN** the conversation still exists, gets a fresh runtime pod, and resumes its session
