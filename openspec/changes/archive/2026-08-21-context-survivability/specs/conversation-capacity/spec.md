## ADDED Requirements

### Requirement: A runtime pod that never starts is reaped

A runtime pod that has not reached a running state within a configured start
deadline SHALL be deleted, and the conversation SHALL be re-admitted only after
a per-conversation backoff that grows with consecutive failures.

The pod SHALL continue to be counted as active for as long as it exists.
Exempting it from the cap SHALL NOT be used as the remedy: the cluster has not
released its resources, so not counting it would provision beyond the cap. The
remedy is that the pod stops existing.

Deletion SHALL free the slot through the same runtime-pod DELETE watch that
already promotes the FIFO-first waiting conversation. No separate scheduling
path SHALL be introduced.

#### Scenario: A pod that cannot attach its volume is reaped

- **WHEN** a runtime pod remains unable to start past the deadline because its volume will not attach
- **THEN** the pod is deleted and its capacity slot becomes available to the FIFO-first waiting conversation

#### Scenario: A stuck pod holds its slot until it is reaped

- **WHEN** a runtime pod is stuck starting and the deadline has not yet passed
- **THEN** it is still counted against the cap, and no additional conversation is admitted in its place

#### Scenario: Repeated failures back off

- **WHEN** a conversation's runtime pod fails to start several times in a row
- **THEN** the interval before the next attempt grows, rather than recreating the pod immediately each time

### Requirement: A blocked runtime start says why, in the kubelet's own words

When a runtime pod fails to start, the conversation SHALL carry a condition
reporting that the runtime did not start, and its message SHALL carry the
evidence from the pod itself — the unmet pod condition and the most recent
related event — rather than only that a deadline elapsed.

A message stating only that a deadline was exceeded SHALL NOT be sufficient. The
failure this requirement exists for was fifteen hours long and produced nothing
an operator could act on, and a generic timeout message would reproduce exactly
that.

The condition SHALL be visible in the console alongside the conversation's
phase, so that a queue which has stopped moving explains itself where an
operator is already looking.

#### Scenario: The reason reaches the operator

- **WHEN** a runtime pod cannot start because its volume will not attach
- **THEN** the conversation reports a runtime-not-started condition naming the attach failure, and the console shows it

#### Scenario: A generic timeout is not enough

- **WHEN** a runtime pod exceeds its start deadline
- **THEN** the recorded reason distinguishes why it could not start, not merely that it did not

### Requirement: Runtime pods release ahead of a node going down

When a node is cordoned or carries a no-schedule taint, the manager SHALL stop
admitting conversations onto that node and SHALL release the runtime pods there
that need no worker, using the same release path as the operator-issued exit
command.

A pod with work inflight SHALL NOT be released, for the reason the exit command
already refuses one: the replacement would re-run work that may already have
acted.

This exists so the last consumer of a shared volume leaves before the node goes
down, letting the volume detach and its filesystem unmount cleanly. It SHALL be
documented as shrinking that window rather than closing it, because the storage
provider chooses where a shared volume is served independently of where runtime
pods are scheduled.

#### Scenario: A cordoned node sheds its idle runtimes

- **WHEN** a node hosting idle runtime pods is cordoned
- **THEN** those pods are released and no new conversation is admitted onto that node

#### Scenario: An inflight run is not interrupted by a cordon

- **WHEN** a node is cordoned while one of its runtime pods has work inflight
- **THEN** that pod is left alone until the run completes

## MODIFIED Requirements

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

A conversation SHALL also be held in `Pending` when the manager is treating
context storage as unavailable, and the phase SHALL report that cause distinctly
from waiting for capacity. An operator looking at a queue that has stopped
moving needs to know whether it is full or whether its storage is gone, and the
two demand different responses. Work SHALL be held rather than failed, and SHALL
proceed once storage is reachable again.

#### Scenario: Pending conversation provisions nothing
- **WHEN** a conversation is created while the cap is full of busy conversations
- **THEN** it has no runtime pod, no chat topic, and no `agentops-mcp-conv-<name>` ConfigMap

#### Scenario: Grouping still reuses a pending conversation
- **WHEN** a second signal arrives with the same signature as a `Pending` conversation inside the grouping window
- **THEN** the signal is appended as an input to that pending conversation rather than opening another one

#### Scenario: Backlog survives a manager restart
- **WHEN** the manager restarts with pending conversations waiting
- **THEN** those conversations are still pending after restart and are still admitted in their original order

#### Scenario: A storage outage is distinguishable from a queue
- **WHEN** conversations are held because context storage is unavailable
- **THEN** their Pending phase reports that cause, distinctly from waiting for capacity
