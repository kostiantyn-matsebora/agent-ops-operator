## MODIFIED Requirements

### Requirement: Every piece of live state has one declared home
The system SHALL classify every piece of live state into exactly one of three
homes, and SHALL NOT hold state that belongs to no class:

- **Kubernetes API** — configuration, conversation state (session id, threads,
  inputs, runs, phase, and the conversation's MESSAGE RECORD — what people sent
  as well as what the agent answered), adapter cursors, and suppression windows.
  Recovered by reading; survives any process restart and any rescheduling.
- **PersistentVolume** — state that genuinely IS a filesystem: the runtime's
  agent session files and, optionally, its repository checkout.
- **Deliberately lossy** — bounded telemetry whose loss costs history, never
  correctness. It SHALL be documented as lossy and SHALL report its gaps.

The manager SHALL mount no PersistentVolume. Manager state either derives from
CR state or is telemetry; binding the manager to a volume would pin it to one
node and defeat rescheduling.

A conversation's messages SHALL NOT fall into the deliberately-lossy class. They
were never declared lossy and were nonetheless lost: the input queue is pruned
once processed, so a conversation kept the answers and dropped the questions,
and a viewer could rebuild only half a thread. What a viewer holds in memory
SHALL be a cache of that record, never its only copy.

#### Scenario: A restarted viewer rebuilds the whole thread
- **WHEN** a viewer restarts and reconnects to a conversation
- **THEN** it rebuilds every message from the Kubernetes API, and only what was
  never CR state — acks and cards composed at delivery time — is missing

#### Scenario: Manager restarts with work in flight
- **WHEN** the manager process restarts while conversations are active
- **THEN** it recovers current state by reading Kubernetes objects alone, mounting no volume and consulting no local file

#### Scenario: No unclassified state
- **WHEN** a component holds state in process memory
- **THEN** that state is either a cache of a Kubernetes object, derivable from Kubernetes objects, or declared lossy telemetry
