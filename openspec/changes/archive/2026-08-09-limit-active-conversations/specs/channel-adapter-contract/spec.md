## MODIFIED Requirements

### Requirement: Outbound operations delivered to adapters by long-poll
The manager SHALL expose `GET /channel/ops?adapter=<name>&wait=<seconds>` (non-leader-gated) returning the next pending outbound operation for any Channel served by that adapter, or 204 on timeout. The parameter names the adapter (the same value Channels carry in `spec.adapter`), replacing the former `?type=`; a request carrying the retired parameter SHALL fail with 400 naming the replacement rather than being served an empty list, so an outdated adapter fails loudly instead of appearing to work while delivering nothing. Operations SHALL carry a stable id, the channel and conversation names, a kind (`ensure-topic`, `send`, or `close-topic`), and a kind-specific payload (`send` includes the message text and target thread id; `close-topic` includes the target thread id and asks the adapter to archive or close that thread on its transport). Operations SHALL be derived from CR state or router actions such that an operation lost in flight (manager restart, adapter crash) is regenerated or safely skipped; delivery is at-least-once and adapters MUST tolerate duplicates by id. A `close-topic` operation SHALL be derivable from CR state for as long as it is outstanding, which the deleting conversation's finalizer guarantees.

#### Scenario: Adapter receives a topic-creation op
- **WHEN** a Conversation referencing a Channel with `adapter: slack` is reconciled with no `threadId` and an adapter is long-polling `/channel/ops?adapter=slack`
- **THEN** the adapter receives an `ensure-topic` operation identifying that conversation

#### Scenario: Adapter receives a topic-close op
- **WHEN** a Conversation bound to a Channel with `adapter: slack` is deleted while holding a thread id
- **THEN** the adapter receives a `close-topic` operation carrying that thread id

#### Scenario: No ops available
- **WHEN** an adapter long-polls and no operation becomes available within `wait`
- **THEN** the manager responds 204 and the adapter re-polls

#### Scenario: Unclaimed op survives a manager restart
- **WHEN** the manager restarts while an `ensure-topic` op is queued but undelivered
- **THEN** reconciliation re-enqueues an equivalent operation and the conversation still gets its topic

#### Scenario: Retired parameter fails loudly
- **WHEN** an adapter built against the old contract polls `/channel/ops?type=slack`
- **THEN** the manager responds 400 naming `adapter` as the expected parameter

### Requirement: Asynchronous operation completion
The manager SHALL expose `POST /channel/ops/{id}/done` accepting the operation result — for `ensure-topic`, the thread id string to store in the conversation's status; for `close-topic`, an empty body on success; for failures, an error the manager records (condition/event) and may retry via regeneration. The Conversation reconciler SHALL tolerate the pending window between enqueue and completion: inputs stay queued, serial-per-conversation semantics hold, and runtime-pod handling proceeds per existing ordering rules. A failed `close-topic` SHALL be logged rather than recorded on the Conversation and SHALL NOT be regenerated after its grace period, because the conversation it belongs to is being deleted and no object remains to carry a condition; an adapter that does not implement the kind therefore leaves the thread open without blocking anything.

#### Scenario: Topic id lands asynchronously
- **WHEN** an adapter completes an `ensure-topic` op with `{threadId: "9876"}`
- **THEN** the conversation's `status.threadId` becomes `"9876"` and dispatch proceeds normally

#### Scenario: Failed op is surfaced, not silently dropped
- **WHEN** an adapter completes an op with an error
- **THEN** the failure is observable on the Conversation (condition or event) and the operation is eligible for regeneration

#### Scenario: Close-topic completes with an empty result
- **WHEN** an adapter archives the thread and completes the `close-topic` op with an empty body
- **THEN** the manager releases the conversation's finalizer and the object is deleted

#### Scenario: Failed close-topic does not block deletion
- **WHEN** an adapter completes a `close-topic` op with an error
- **THEN** the failure is logged, no Conversation condition is written, and deletion proceeds
