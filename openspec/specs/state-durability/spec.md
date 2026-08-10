# state-durability Specification

## Purpose
TBD - created by archiving change persistence-in-chart. Update Purpose after archive.
## Requirements
### Requirement: Every piece of live state has one declared home
The system SHALL classify every piece of live state into exactly one of three
homes, and SHALL NOT hold state that belongs to no class:

- **Kubernetes API** — configuration, conversation state (session id, threads,
  inputs, runs, phase), adapter cursors, and suppression windows. Recovered by
  reading; survives any process restart and any rescheduling.
- **PersistentVolume** — state that genuinely IS a filesystem: the runtime's
  agent session files and, optionally, its repository checkout.
- **Deliberately lossy** — bounded telemetry whose loss costs history, never
  correctness. It SHALL be documented as lossy and SHALL report its gaps.

The manager SHALL mount no PersistentVolume. Manager state either derives from
CR state or is telemetry; binding the manager to a volume would pin it to one
node and defeat rescheduling.

#### Scenario: Manager restarts with work in flight
- **WHEN** the manager process restarts while conversations are active
- **THEN** it recovers current state by reading Kubernetes objects alone, mounting no volume and consulting no local file

#### Scenario: No unclassified state
- **WHEN** a component holds state in process memory
- **THEN** that state is either a cache of a Kubernetes object, derivable from Kubernetes objects, or declared lossy telemetry

### Requirement: Outbound operations other than close-topic are derivable from CR state
Every outbound channel operation except `close-topic` SHALL be re-derivable from
Kubernetes state after a manager restart. A run whose result is recorded in
`Conversation.status` but not yet delivered to a bound thread SHALL be
re-enqueued by reconciliation. The in-memory operation queue SHALL remain the
hot path and SHALL NOT become the record of what is owed.

`close-topic` SHALL keep its terminal semantics: it is not regenerated, because
the object that would carry the obligation is being deleted.

#### Scenario: Reply survives a restart between completion and delivery
- **WHEN** the manager restarts after `POST /work/done` recorded a run result but before any adapter claimed the resulting `send` op
- **THEN** reconciliation re-enqueues the reply and the bound threads receive it exactly as if no restart had happened

#### Scenario: Delivered replies are not re-posted
- **WHEN** the manager restarts after a run's reply was delivered to every bound thread
- **THEN** no `send` op is regenerated for that run and no thread receives a duplicate

#### Scenario: Partial delivery completes rather than repeats
- **WHEN** a run's reply reached one of two bound threads before a restart
- **THEN** only the undelivered thread receives a `send` op after recovery

#### Scenario: Upgrading does not re-post history
- **WHEN** the manager is upgraded to a version that tracks delivery and first observes conversations whose runs completed before it started
- **THEN** those runs are recorded as delivered without enqueueing any `send`, and no bound thread receives an old answer again

### Requirement: Suppression windows survive a manager restart
Fingerprint cooldown state SHALL be durable for the lifetime of its window. The
manager SHALL record suppression on the owning `SignalSource` and SHALL load it
before applying cooldown to a source after a restart. Entries older than their
window SHALL be pruned so the record stays bounded.

#### Scenario: Cooldown holds across a restart
- **WHEN** the manager restarts and an adapter re-delivers a signal whose fingerprint is inside the source's cooldown window
- **THEN** the signal is suppressed and no duplicate conversation is opened

#### Scenario: Expired entries do not accumulate
- **WHEN** suppression entries age past their window
- **THEN** they are removed from the `SignalSource` and the recorded state stays bounded

### Requirement: Telemetry gaps are reported, never rendered as silence
The activity ring SHALL remain bounded, in-memory and lossy. A client whose
cursor cannot be served — because it was evicted, or because it predates the
current manager process — SHALL be told to resync. A viewer SHALL render such a
boundary as a gap in history, and SHALL NOT present it as a period in which
nothing happened.

#### Scenario: Cursor predates the current process
- **WHEN** a console reconnects after a manager restart with a cursor the new process cannot serve
- **THEN** the response marks a resync and the console shows an explicit gap rather than an empty timeline

#### Scenario: Losing telemetry costs no correctness
- **WHEN** the entire activity ring is lost
- **THEN** conversations, deliveries and suppression are unaffected and only history is missing

### Requirement: A restart-resilience matrix is documented and maintained
The documentation SHALL carry a matrix naming every component, the state it
holds, that state's declared home, and what a restart of that component costs.
Adding state to a component SHALL require adding its row.

#### Scenario: Guarantee is checkable
- **WHEN** an operator asks what restarting a given component loses
- **THEN** the answer is read from the documented matrix rather than inferred from code

