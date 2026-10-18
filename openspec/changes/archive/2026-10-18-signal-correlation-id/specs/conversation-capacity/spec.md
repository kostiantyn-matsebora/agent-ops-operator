# conversation-capacity — delta

## MODIFIED Requirements

### Requirement: Over-cap conversations wait in a Pending phase
`ConversationPhase` SHALL include `Pending`, meaning the conversation exists and
holds its inputs and wiring snapshot but has not been admitted.

While a conversation is `Pending` the manager SHALL NOT create a runtime pod,
SHALL NOT enqueue any `ensure-topic` operation, and SHALL NOT compile or create
the conversation's MCP ConfigMap.

Its inputs, `conversationId`, and pipeline wiring snapshot SHALL be retained
unchanged, so id-equality routing treats it exactly like an admitted
conversation.

`Queued` SHALL keep its existing meaning — admitted, with work waiting behind
the serial-per-conversation rule — and SHALL NOT be used for capacity waiting.

A conversation SHALL also be held in `Pending` when the manager is treating
context storage as unavailable, and the phase SHALL report that cause distinctly
from waiting for capacity.

An operator looking at a queue that has stopped moving needs to know whether it
is full or whether its storage is gone, and the two demand different responses.
Work SHALL be held rather than failed, and SHALL proceed once storage is
reachable again.

#### Scenario: Pending conversation provisions nothing
- **WHEN** a conversation is created while the cap is full of busy conversations
- **THEN** it has no runtime pod, no chat topic, and no `agentops-mcp-conv-<name>` ConfigMap

#### Scenario: Grouping still reuses a pending conversation
- **WHEN** a second signal arrives whose `initiationId` equals a `Pending` conversation's `conversationId`, outside the source's cooldown
- **THEN** the signal is appended as an input to that pending conversation rather than opening another one

#### Scenario: Backlog survives a manager restart
- **WHEN** the manager restarts with pending conversations waiting
- **THEN** those conversations are still pending after restart and are still admitted in their original order

#### Scenario: A storage outage is distinguishable from a queue
- **WHEN** conversations are held because context storage is unavailable
- **THEN** their Pending phase reports that cause, distinctly from waiting for capacity
