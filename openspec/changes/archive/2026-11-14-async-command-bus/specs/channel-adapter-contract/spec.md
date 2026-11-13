## REMOVED Requirements

### Requirement: Outbound operations delivered to adapters by long-poll
**Reason**: An outbound operation is no longer taken by the read. It is read, acknowledged, then reported, and the manager serves any replica, so the leader-only rejection has no cause left.

**Migration**: Read the replacement requirement below. An adapter now acknowledges each operation before acting on it, and a `503` is only ever a transient answer to retry as a `204`.

### Requirement: Adapters absorb retryable backpressure inside the claim window
**Reason**: There is no claim window. The acknowledgement starts a delivery TTL, and the manager never redelivers an acknowledged operation.

**Migration**: Read the replacement requirement below. The retry budget is bounded by the acknowledgement's `deliveryTtlSeconds`.

## MODIFIED Requirements

### Requirement: Asynchronous operation completion
The manager SHALL expose `POST /channel/ops/{id}/done` accepting the report of an acknowledged operation. For `ensure-topic` it is the thread id string to store in the conversation's status. For `close-topic` it is an empty body on success. For failures it is an error the manager records. A failure SHALL be final: the manager SHALL NOT redeliver the operation, and what is still owed is re-derived from conversation state only after the kind's cool-off.

The Conversation reconciler SHALL tolerate the pending window between enqueue and report: inputs stay queued, serial-per-conversation semantics hold, and runtime-pod handling proceeds per existing ordering rules.

A failed `close-topic` or `delete-conversation` is the exception to the failure rule above: it SHALL be logged and SHALL write no Conversation condition. A failed `close-topic` SHALL NOT be exempt from re-derivation.

The conversation survives its close. The thread stays absent from `status.threadsArchived[]`, so the archive is still owed and a later reconciliation re-derives the op.

A failed `delete-conversation` is logged and not re-derived. No object remains to carry the obligation.

#### Scenario: Topic id lands asynchronously
- **WHEN** an adapter reports an `ensure-topic` op done with `{threadId: "9876"}`
- **THEN** that channel's binding in the conversation's `status.threads[]` carries `"9876"` and dispatch proceeds normally

#### Scenario: Failed op is surfaced, not silently dropped
- **WHEN** an adapter reports an op failed with an error
- **THEN** the failure is observable on the Conversation (condition or event), is final, and the operation is eligible for re-derivation after the cool-off

#### Scenario: Close-topic completes with an empty result
- **WHEN** an adapter archives the thread and reports the `close-topic` op done with an empty body
- **THEN** the thread is recorded in `status.threadsArchived[]`

#### Scenario: Failed close-topic is re-derived
- **WHEN** an adapter reports a `close-topic` op failed
- **THEN** the thread stays absent from `status.threadsArchived[]` and a later reconciliation, after the cool-off, enqueues the op again

#### Scenario: Failed close-topic is logged without a condition
- **WHEN** an adapter reports a `close-topic` op failed
- **THEN** the failure is logged, no Conversation condition is written, and a later reconciliation re-derives the op while the object exists

#### Scenario: Failed delete-conversation is not regenerated
- **WHEN** an adapter reports a `delete-conversation` op failed
- **THEN** the failure is logged and no op is regenerated

## ADDED Requirements

### Requirement: An adapter reads, acknowledges, then reports each outbound operation
The manager SHALL expose `GET /channel/ops?adapter=<name>&contract=<version>&wait=<seconds>`. It SHALL offer the next outbound operation for any Channel served by that adapter and answer 204 on timeout. A read SHALL change nothing, and any manager replica SHALL answer it.

The adapter SHALL acknowledge an operation with `POST /channel/ops/{id}/ack?adapter=<name>` before acting on it. The reply SHALL be `{ok, deliveryTtlSeconds, consumed}`. It SHALL answer `404` when the operation is gone and `409` when it is held but not offered. The acknowledgement SHALL be idempotent.

From the acknowledgement the operation SHALL be the adapter's: the manager SHALL NOT send it again, and a delivery TTL SHALL start, declared per kind (stable 10 minutes, ephemeral 2 minutes, topic operations 5 minutes, `delete-conversation` 5 minutes). The adapter SHALL then report the operation done or failed with `POST /channel/ops/{id}/done`. An operation with no report when its TTL passes SHALL be failed by the manager, and a failure SHALL be final.

The parameter names the adapter, the same value Channels carry in `spec.adapter`, replacing the former `?type=`.

A request carrying the retired parameter SHALL fail with 400 naming the replacement, rather than being served an empty list. The polling adapter SHALL additionally declare the outbound contract version it speaks in `contract=<version>`. An absent or unsupported declaration SHALL fail with 400 naming what is expected.

```
GET  /channel/ops?adapter=slack&contract=2&wait=25
200  {"id":"send:conv-1:chat-ops:run-7","channel":"chat-ops","kind":"send","message":{...}}
POST /channel/ops/send:conv-1:chat-ops:run-7/ack?adapter=slack
200  {"ok":true,"deliveryTtlSeconds":600,"consumed":false}
POST /channel/ops/send:conv-1:chat-ops:run-7/done   {}
200  {"ok":true}
```

Every operation SHALL carry a stable id, the channel and conversation names, a kind, and a kind-specific **structured** payload, never pre-rendered display text:

| Kind | Payload |
|---|---|
| `send` | a typed message (`signal`, `answer`, `relay`, or `notice`) with markdown-valued free text and typed structured fields, plus the target thread id |
| `ensure-topic` | a topic descriptor (`conversation`, `pipeline?`, `source?`, `title`, `labels`, `kind`), never a rendered title string. `pipeline` is inferred and MAY be empty |
| `close-topic` | the target thread id, asking the adapter to archive or close that thread on its transport |
| `delete-conversation` | the target thread id and the notice, reporting that the conversation ended for good |

Escaping, length limits, chunking, truncation, and thread naming SHALL be the adapter's responsibility. The manager SHALL emit no transport markup and declare no maximum message size.

Operations SHALL be derived from CR state or router actions, such that an operation lost in flight is re-derived or safely skipped. Adapters MUST treat an id as opaque and tolerate an operation offered more than once before it is acknowledged.

#### Scenario: Adapter receives a topic-creation op
- **WHEN** a Conversation referencing a Channel with `adapter: slack` is reconciled with no `threadId` and an adapter is long-polling `/channel/ops?adapter=slack`
- **THEN** the adapter is offered an `ensure-topic` operation identifying that conversation, carrying a descriptor it names the thread from

#### Scenario: Adapter receives a topic-close op
- **WHEN** a Conversation bound to a Channel with `adapter: slack` is closed while holding a thread id
- **THEN** the adapter is offered a `close-topic` operation carrying that thread id

#### Scenario: No ops available
- **WHEN** an adapter long-polls and no operation becomes available within `wait`
- **THEN** the manager responds 204 and the adapter re-polls

#### Scenario: A read is not a hand-over
- **WHEN** an adapter reads an operation and stops before acknowledging it
- **THEN** the next read offers the same operation, since no delivery had begun

#### Scenario: Unacknowledged op survives a manager restart
- **WHEN** the manager restarts while an `ensure-topic` op is queued but not acknowledged
- **THEN** the operation is still offered, or reconciliation re-enqueues an equivalent one, and the conversation still gets its topic

#### Scenario: Retired parameter fails loudly
- **WHEN** an adapter built against the old contract polls `/channel/ops?type=slack`
- **THEN** the manager responds 400 naming `adapter` as the expected parameter

#### Scenario: Outdated outbound contract fails loudly
- **WHEN** an adapter that expects string-valued `send` ops polls for operations
- **THEN** the manager responds 400 naming the required contract version, rather than delivering messages it would post as empty text

#### Scenario: Send ops carry meaning, not markup
- **WHEN** a `send` op is delivered for an agent's answer
- **THEN** it carries an `answer` message with a markdown body and no transport markup, and the adapter renders it

#### Scenario: An adapter that never acknowledges is offered the op forever
- **WHEN** an adapter built against the previous contract reads an op and never acknowledges it
- **THEN** the manager offers the same op on every read and the adapter delivers nothing it can be held to, which is why adapters upgrade together with the manager

#### Scenario: A failed op is final
- **WHEN** an adapter reports an acknowledged op failed, or never reports it before its TTL
- **THEN** the manager does not redeliver it, and what is owed is re-derived after the kind's cool-off

### Requirement: Adapters absorb retryable backpressure inside the delivery TTL
An adapter SHALL distinguish **retryable** transport conditions, such as rate limits, timeouts, and transient server errors, from **terminal** ones. It SHALL retry retryable conditions in-process before reporting the operation.

Only a terminal failure, or exhaustion of the adapter's retry budget, SHALL be reported as an error to `POST /channel/ops/{id}/done`.

When the transport states how long to wait, the adapter SHALL honor that value rather than applying its own backoff. The adapter's total in-process retry budget for one operation SHALL remain strictly below the `deliveryTtlSeconds` the acknowledgement stated, leaving room to report.

The budget is PER OPERATION, never per transport call. An operation that makes several calls, such as a reopen plus a create, one send per chunk, or a fallback, SHALL charge every call's wait, retry and own timeout against the one budget.

An adapter that overruns the TTL is failed by the manager while it may still be delivering, and a failed operation is not offered again. Reporting a retryable condition as an operation failure gives up the delivery for a condition the adapter could have ridden out, and adapters SHALL NOT treat it as the normal path.

#### Scenario: Rate limit is slept out rather than reported
- **WHEN** the transport rejects a `send` with a retryable rate-limit error and a stated wait
- **THEN** the adapter waits the stated interval, retries the same operation, and reports success without the manager observing a failure

#### Scenario: Retry budget stays under the delivery TTL
- **WHEN** an operation's retries would exceed the acknowledged delivery TTL
- **THEN** the adapter abandons the retry and reports the operation failed while the TTL still holds, rather than the manager failing it at the TTL

#### Scenario: A multi-call operation shares one budget
- **WHEN** an operation requires several transport calls and each is rate-limited in turn
- **THEN** the summed waits and timeouts stay below the delivery TTL, or the adapter reports failure while it still holds

#### Scenario: Terminal error is reported immediately
- **WHEN** the transport rejects an operation for a reason retrying cannot fix, such as a missing thread or a rejected credential
- **THEN** the adapter reports the failure without retrying and the manager applies its own recovery rules for that operation kind

### Requirement: Receipt-only adapters acknowledge on receipt
The adapters named by the manager's `CHANNEL_RECEIPT_ADAPTERS` (the console by default) SHALL be the declared exception to the delivery TTL. Their acknowledgement SHALL be only their receipt of the operation from the manager, answered `consumed: true` with no delivery TTL. They SHALL NOT report, and the manager SHALL perform the delivery side effects and delete the operation at the acknowledgement.

A receipt adapter MAY read `GET /channel/ops/stream`, which pushes each unacknowledged operation once per connection and offers again, after a reconnect, exactly the operations never acknowledged. The stream SHALL have no cursor. An adapter outside the set SHALL be answered `404`.

The exception SHALL be configuration and never code: a new adapter SHALL need no manager change, and SHALL be receipt-only only by being named.

The manager SHALL assign a receipt adapter's thread id when it enqueues `ensure-topic`.

#### Scenario: A receipt adapter reports nothing
- **WHEN** the console acknowledges an op
- **THEN** the reply carries `consumed: true`, the manager deleted the op, and no `/done` call follows

#### Scenario: A reconnect is offered only what was never acknowledged
- **WHEN** the console's stream drops after acknowledging some ops and not others, and it reconnects
- **THEN** it is pushed exactly the ops it never acknowledged

#### Scenario: Another adapter has no stream
- **WHEN** an adapter outside the receipt set requests the stream
- **THEN** the manager answers `404`
