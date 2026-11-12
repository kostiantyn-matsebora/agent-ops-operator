## MODIFIED Requirements

### Requirement: Outbound operations other than delete-conversation are derivable from CR state
Every outbound channel operation except `delete-conversation` SHALL be re-derivable from
Kubernetes state after a manager restart. A run whose result is recorded in
`Conversation.status` but not yet delivered to a bound thread SHALL be
re-enqueued by reconciliation.

The command bus SHALL be the hot path. The record of what is owed SHALL be
the Conversation's own `status`, not the bus. The bus's logs are bounded
telemetry and SHALL NOT be the only place an owed operation is recorded.

Re-derivation SHALL NOT depend on a restart. The bus's deduplication SHALL
record operations that **succeeded**, never operations that were merely
**attempted**.

While a derivable operation is unread, or acknowledged and within its delivery
TTL, its id SHALL stay pending, so a re-derive is suppressed. The adapter, not the
bus, retries delivery against its surface until that TTL.

When the operation fails, whether the adapter reports it or its delivery TTL
passes with no report, the failure is final: the bus SHALL park its id for a
cool-off and then release it. The next reconciliation after the release
re-derives it.

An operation whose failure leaves its id suppressed forever is
indistinguishable from one that was delivered. That would convert a
transient transport error into permanent, unrecoverable loss of a reply
the CR still records as owed.

`delete-conversation` SHALL keep its terminal semantics: it is not regenerated,
because the object that would carry the obligation is being deleted. It is the
only exemption. It SHALL never be parked.

`close-topic` SHALL NOT be exempt. It once was, because closing a
conversation used to delete it. The object now survives its close, and
`status.threadsArchived[]` records which threads are done — so an
unarchived bound thread is an archive still owed, and is re-derivable like
any other operation.

A reply that remains undelivered to a bound thread after its operation failed
SHALL be observable on the Conversation rather than only in manager logs, as
`status.threads[].undeliveredReply`: the run id whose reply is still owed to
that thread, cleared when delivery succeeds.

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
- **THEN** the failure is final, the op's id is parked and then released, and reconciliation re-derives the reply so it reaches the thread without operator intervention

#### Scenario: A dead reply is re-derived after its cool-off
- **WHEN** a `send` op for a run reply fails, by a report or by its delivery TTL
- **THEN** its id is parked, then released, and reconciliation re-enqueues the same stable op id while the reply is still owed

#### Scenario: Failed opening card is re-derived without a restart
- **WHEN** an adapter's input `signal` card op fails
- **THEN** the card is re-derived once its id is released, because a card is derivable from the conversation's inputs and carries no CR-side delivery marker of its own

#### Scenario: Rate-limited burst leaves no thread permanently empty
- **WHEN** a transport rejects a batch of `ensure-topic` and `send` operations with a retryable error and later accepts them
- **THEN** every created thread eventually carries both its opening card and its run replies, and no conversation is left with a thread that has a recorded result but no posted message

#### Scenario: Failed close-topic is re-derived like any other operation
- **WHEN** a `close-topic` op fails and its conversation still exists
- **THEN** its id is released after the cool-off and reconciliation re-derives the op, because the thread is still owed an archive

#### Scenario: Failed delete-conversation is still not regenerated
- **WHEN** a `delete-conversation` op completes with an error while the conversation's finalizer is releasing
- **THEN** the op is not re-derived and the finalizer releases regardless, because the object that would carry the obligation is gone

#### Scenario: An owed reply is visible on the object
- **WHEN** a run's reply has failed delivery to a bound thread and has not yet succeeded
- **THEN** the Conversation reports the undelivered thread in `status.threads[].undeliveredReply`, so an empty chat thread can be diagnosed without reading manager logs

#### Scenario: A claim held only in a dead process's memory is not the record of anything
- **WHEN** an adapter reads an `ensure-topic` op and the adapter or a manager replica dies before the acknowledgement
- **THEN** the op was never handed over, and the next read on any replica offers it again

#### Scenario: An acknowledged message whose adapter dies is failed at its TTL
- **WHEN** an adapter acknowledges an `ensure-topic` op and dies before reporting
- **THEN** the manager does not send it again, fails it when its delivery TTL passes, and reconciliation re-derives it once its id is released

### Requirement: A restart-resilience matrix is documented and maintained
The documentation, in `docs/concepts.md`, SHALL carry a matrix naming every component, the state it
holds, that state's declared home, and what a restart of that component costs.
Adding state to a component SHALL require adding its row.

A row whose home is the state store SHALL say what each shipped backend does
on a restart of the backend itself, since that is a restart the Kubernetes
API never had.

The matrix SHALL name `status.threads[].undeliveredReply`: a state store
record, surviving every restart.

The matrix SHALL name a channel op as a message in the command bus's log, and its
acknowledgement as the hand-over that starts its delivery TTL. For it the matrix SHALL say:

- the log is bounded telemetry
- a message nobody acknowledged is offered again after a manager restart, and an acknowledged one is failed when its delivery TTL passes
- what a restart of the bus server itself costs under each shipped bus backend

The matrix SHALL name the `Signal` record — its spec, its claimants and its processing state (`status.attempts`, `status.consumedBy`):

- state store records, surviving every restart and leader change
- a leader change costs no accepted signal
- the new leader resumes every non-terminal `Signal` from the state store, redoing only the interrupted pass idempotently, so a conversation or input that already landed is found and never written twice

#### Scenario: Guarantee is checkable
- **WHEN** an operator asks what restarting a given component loses
- **THEN** the answer is read from the documented matrix rather than inferred from code

#### Scenario: Claim and owed-reply state have rows
- **WHEN** an operator asks what a manager restart does to an acknowledged op or an undelivered reply
- **THEN** the matrix names the op as a message in the command bus's log, handed over at its acknowledgement, and the reply as a conversation record in the state store, surviving the restart

#### Scenario: A backend restart has a row
- **WHEN** an operator asks what restarting the state backend or the bus server costs
- **THEN** the matrix answers per shipped backend, and the `kube` row says
  "the Kubernetes API's own guarantee"

#### Scenario: Accepted signals have a row
- **WHEN** an operator asks what a manager restart or leader change does to a signal accepted but not yet fully routed
- **THEN** the matrix names the `Signal` record as a state store record, resumed by the new leader, which redoes only the interrupted pass and replays no routed work

## ADDED Requirements

### Requirement: The queue logs and dead-letter record are declared bounded lossy telemetry
The command bus's queue logs and dead-letter record SHALL be classified in
the deliberately lossy home. They SHALL be documented as lossy and SHALL
report their gaps.

Their loss SHALL cost task history and work not yet claimed, never
conversation correctness. Every command's effect SHALL land on a record in the
state store or the Kubernetes API, and that record SHALL stay the only durable
truth.

Both SHALL be bounded by retention, in age and size: an unconsumed message is kept 7 days by default, and dead letters 7 days or 10000 records.

A feed consumer whose point was trimmed past SHALL be told to resync and SHALL render the boundary as a gap in history, never as a period in which nothing happened.

The manager SHALL still mount no PersistentVolume. A bus server's own storage
belongs to the bus server and not to the manager.

#### Scenario: Losing the log costs no correctness
- **WHEN** the entire task log and dead-letter record are lost
- **THEN** conversations, deliveries, signals and suppression are unaffected, and only task history and work not yet claimed are missing

#### Scenario: A trimmed feed is reported as a gap
- **WHEN** a task-stream consumer resumes from a point the log no longer retains
- **THEN** it is told to resync and shows a gap, not silence

#### Scenario: An accepted command's effect is recoverable from state
- **WHEN** a task goes dead after its effect partly landed
- **THEN** the conversation record shows what landed and reconciliation derives what is still owed
