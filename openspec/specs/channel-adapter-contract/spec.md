# channel-adapter-contract

## Purpose

The HTTP contract between the manager and out-of-process channel adapters: outbound op delivery, async completion, inbound routing, config/state/status serving, and auth.
## Requirements
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

### Requirement: Inbound messages enter through the shared router
The manager SHALL expose `POST /channel/inbound` accepting `{channel, threadId, text, sender?}` as the CONTINUATION path only: `threadId` is REQUIRED, and the message SHALL be routed through the transport-neutral router as a reply input on the matching conversation (busy-ack preserved). Resulting acks SHALL flow back to the adapter as `send` operations, and the message SHALL be relayed to the conversation's sibling channels as attributed text. In-process (registry) providers SHALL use the same operation pipeline so routing behavior is identical for built-in and external types.

The ORIGINATION branch is removed: command parsing and default-profile conversation creation no longer occur here, and a message in an unrecognized thread is no longer adopted as a new conversation. A request without `threadId` SHALL be rejected with a message naming the signal path as the origination route. Channel implementations MAY omit inbound entirely when a separate component handles their ingest.

#### Scenario: Threaded reply is queued serially
- **WHEN** an adapter posts an inbound message whose thread id matches a conversation with an inflight unit
- **THEN** a reply input is appended (not dispatched concurrently) and a busy-ack `send` op is emitted

#### Scenario: Missing thread id is refused, not originated
- **WHEN** an adapter posts an inbound message with no thread id
- **THEN** the request is rejected with a message naming the signal path, and no Conversation is created

#### Scenario: Unknown thread is not adopted
- **WHEN** an adapter posts an inbound message whose thread id matches no conversation
- **THEN** no conversation is created or adopted

#### Scenario: Reply still mirrors to sibling channels
- **WHEN** a reply arrives on one channel of a multi-channel conversation
- **THEN** it is relayed to the sibling channels as attributed text, unchanged from before

### Requirement: Adapters need no Kubernetes access
The contract SHALL carry everything an adapter needs beyond ops and inbound: `GET /channel/channels?adapter=<t>` returns the channels of that type with their opaque `spec.config` **and a `credentialEnvPrefix` locating each channel's projected credentials in the adapter's own environment (Secret key `K` is env `<prefix>K`)**; `GET/PUT /channel/state/{channel}/{key}` persists adapter cursor state (manager-side, as Channel annotations) across adapter restarts; `POST /channel/channels/{name}/status` sets the Channel's Ready condition (adapters report their own config validation results there). The prefix SHALL be derived from projection metadata (the Channel name), never from Secret values or key enumeration.

#### Scenario: Adapter reads its config through the contract
- **WHEN** an adapter lists `GET /channel/channels?adapter=telegram`
- **THEN** it receives each `adapter: telegram` Channel's name and raw `spec.config` without any Kubernetes API access

#### Scenario: Adapter locates per-channel credentials
- **WHEN** a served Channel has `credentialsSecretRef` projected under prefix `AGENTOPS_CRED_HOME_OPS_`
- **THEN** the channel listing entry carries `credentialEnvPrefix: "AGENTOPS_CRED_HOME_OPS_"` and the adapter resolves Secret key `botToken` as env `AGENTOPS_CRED_HOME_OPS_botToken`

#### Scenario: Cursor state survives adapter restart
- **WHEN** an adapter PUTs state key `offset` and later restarts and GETs it
- **THEN** the previously written value is returned

#### Scenario: Invalid config surfaces on the Channel
- **WHEN** an adapter reports `ready: false` with a reason for a misconfigured Channel
- **THEN** the Channel's status carries a False Ready condition with that reason

### Requirement: Adapter endpoints are authenticated without manager secret reads
All `/channel/*` endpoints SHALL require a bearer token that the manager receives via its own deployment environment (no Secret API reads by the manager — the manager SHALL perform zero secret reads after this change). Requests with a missing or invalid token SHALL receive 401. Comparison SHALL be constant-time. In addition to the master token (full scope, hand-deployed adapters), the manager SHALL accept per-adapter tokens derived as `HMAC(masterKey, adapter name)` — validated by re-derivation (stateless, no storage) and **scoped to the routing key the adapter serves, which is its name**: a per-adapter token presented for another key's ops, state, or status SHALL receive 403.

#### Scenario: Valid adapter token
- **WHEN** an adapter calls any `/channel/*` endpoint with the shared bearer token
- **THEN** the request is served

#### Scenario: Missing or wrong token
- **WHEN** a `/channel/*` request lacks the token or presents a wrong one
- **THEN** the manager responds 401 without processing the operation or message

#### Scenario: Token validation survives manager restart
- **WHEN** the manager restarts and an adapter re-polls with its previously issued derived token
- **THEN** the token validates by re-derivation with no Secret reads or stored state

#### Scenario: Per-adapter token is scoped to its name
- **WHEN** the `slack` adapter's derived token is used to poll `/channel/ops?adapter=telegram`
- **THEN** the manager responds 403

