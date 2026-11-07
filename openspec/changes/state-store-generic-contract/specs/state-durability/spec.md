## MODIFIED Requirements

### Requirement: Every piece of live state has one declared home
The system SHALL classify every piece of live state into exactly one of four
homes, and SHALL NOT hold state that belongs to no class:

- **Kubernetes API** — configuration: every kind an operator writes.
  Recovered by reading. Survives any process restart and any rescheduling.
- **State store** — conversation state: `runtimeContextId`, threads, inputs, runs,
  phase, the conversation's MESSAGE RECORD (what people sent as well as what
  the agent answered), signals with their per-claimant routing state, each
  source's cooldown window and its receipt counters, and the key-value state
  an adapter persists through the manager (poll offsets, webhook
  registrations). Reached only through the state store contract, by every
  component that reads it. The selected backend decides where it is kept. The `kube` backend keeps it
  in the Kubernetes API, so a default install has the same two homes it
  always had.
- **PersistentVolume** — state that genuinely IS a filesystem: the runtime's
  agent session files and, optionally, its repository checkout.
- **Deliberately lossy** — bounded telemetry whose loss costs history, never
  correctness. It SHALL be documented as lossy and SHALL report its gaps.

The manager SHALL mount no PersistentVolume. Manager state either derives
from a Kubernetes object, is a cache of a state store record, or is
telemetry.

Binding the manager to a volume would pin it to one node and defeat
rescheduling. A state backend is a separate component for the same reason.

A conversation's messages SHALL NOT fall into the deliberately-lossy class.
They were never declared lossy and were nonetheless lost: the input queue
was pruned once processed, so a conversation kept the answers and dropped
the questions.

What a viewer holds in memory SHALL be a cache of that record, never its
only copy.

#### Scenario: A restarted viewer rebuilds the whole thread
- **WHEN** a viewer restarts and reconnects to a conversation
- **THEN** it rebuilds every message from the state store's record, and only
  what was never a record — acks and cards composed at delivery time — is
  missing

#### Scenario: Manager restarts with work in flight
- **WHEN** the manager process restarts while conversations are active
- **THEN** it recovers current state by reading Kubernetes objects and the
  state store alone, mounting no volume and consulting no local file

#### Scenario: No unclassified state
- **WHEN** a component holds state in process memory
- **THEN** that state is a cache of a Kubernetes object, a cache of a state
  store record, derivable from either, or declared lossy telemetry

#### Scenario: The home does not move with the backend
- **WHEN** an install selects a different state backend
- **THEN** every row of conversation state keeps the same declared home, the
  state store, and only the backend named beside it changes
