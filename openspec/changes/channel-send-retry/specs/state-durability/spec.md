## MODIFIED Requirements

### Requirement: Outbound operations other than close-topic are derivable from CR state
Every outbound channel operation except `close-topic` SHALL be re-derivable from
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

`close-topic` SHALL keep its terminal semantics: it is not regenerated, because
the object that would carry the obligation is being deleted. It is the only
exemption.

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

#### Scenario: Failed close-topic is still not regenerated
- **WHEN** a `close-topic` op completes with an error while the conversation's finalizer is releasing
- **THEN** the op is not re-derived and the finalizer releases regardless, leaving the thread open

#### Scenario: An owed reply is visible on the object
- **WHEN** a run's reply has failed delivery to a bound thread and has not yet succeeded
- **THEN** the Conversation reports the undelivered thread in its status, so an empty chat thread can be diagnosed without reading manager logs
