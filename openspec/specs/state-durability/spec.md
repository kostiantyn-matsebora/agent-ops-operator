# state-durability Specification

## Purpose

The rule that every piece of live state has ONE declared home, and the matrix
that records where each one is.

Manager state is a cache of a Kubernetes object, DERIVABLE from Kubernetes
objects, or declared lossy telemetry — and state fitting none of the three is a
defect. That is why outbound operations are derivable rather than queued
in memory, why suppression windows are written on the SignalSource, and why a
telemetry gap is reported rather than rendered as silence.
## Requirements
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

### Requirement: Outbound operations other than delete-conversation are derivable from CR state
Every outbound channel operation except `delete-conversation` SHALL be re-derivable from
Kubernetes state after a manager restart. A run whose result is recorded in
`Conversation.status` but not yet delivered to a bound thread SHALL be
re-enqueued by reconciliation. The in-memory operation queue SHALL remain the
hot path and SHALL NOT become the record of what is owed.

Re-derivation SHALL NOT depend on a restart. The manager's completed-operation
window exists to suppress duplicates and SHALL therefore record operations that
**succeeded**, never operations that were merely **attempted**. When a derivable
operation completes with an error, the manager SHALL release that operation's
dedup entry so the next reconciliation re-derives it. An operation whose failure
leaves its id in the window is indistinguishable from one that was delivered,
which converts a transient transport error into permanent, unrecoverable loss of
a reply the CR still records as owed.

`delete-conversation` SHALL keep its terminal semantics: it is not regenerated,
because the object that would carry the obligation is being deleted. It is the
only exemption.

`close-topic` SHALL NOT be exempt. It was, while closing a conversation deleted
it; the object now survives its close and `status.threadsArchived[]` records
which threads are done, so an unarchived bound thread is an archive still owed
and is re-derivable like any other operation.

A reply that remains undelivered to a bound thread after its operation failed
SHALL be observable on the Conversation rather than only in manager logs.

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

#### Scenario: Failed reply is re-derived without a restart
- **WHEN** an adapter reports a `send` op for a run reply as failed and the manager keeps running
- **THEN** the operation's dedup entry is released and the next reconciliation re-enqueues the same stable op id, so the reply reaches the thread without operator intervention

#### Scenario: Failed opening card is re-derived without a restart
- **WHEN** an adapter reports a conversation's input `signal` card op as failed
- **THEN** the card is re-derived on the next reconciliation, because a card is derivable from the conversation's inputs and carries no CR-side delivery marker of its own

#### Scenario: Rate-limited burst leaves no thread permanently empty
- **WHEN** a transport rejects a batch of `ensure-topic` and `send` operations with a retryable error and later accepts them
- **THEN** every created thread eventually carries both its opening card and its run replies, and no conversation is left with a thread that has a recorded result but no posted message

#### Scenario: Failed close-topic is re-derived like any other operation
- **WHEN** a `close-topic` op completes with an error and its conversation still exists
- **THEN** the dedup entry is released and reconciliation re-derives the op, because the thread is still owed an archive

#### Scenario: Failed delete-conversation is still not regenerated
- **WHEN** a `delete-conversation` op completes with an error while the conversation's finalizer is releasing
- **THEN** the op is not re-derived and the finalizer releases regardless, because the object that would carry the obligation is gone

#### Scenario: An owed reply is visible on the object
- **WHEN** a run's reply has failed delivery to a bound thread and has not yet succeeded
- **THEN** the Conversation reports the undelivered thread in its status, so an empty chat thread can be diagnosed without reading manager logs

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

