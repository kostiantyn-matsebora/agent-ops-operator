# channel-adapter-contract

## Purpose

The HTTP contract between the manager and out-of-process channel adapters: outbound op delivery, async completion, inbound routing, config/state/status serving, and auth.
## Requirements
### Requirement: Outbound operations delivered to adapters by long-poll
The manager SHALL expose `GET /channel/ops?adapter=<name>&wait=<seconds>` (non-leader-gated) returning the next pending outbound operation for any Channel served by that adapter, or 204 on timeout. The parameter names the adapter (the same value Channels carry in `spec.adapter`), replacing the former `?type=`; a request carrying the retired parameter SHALL fail with 400 naming the replacement rather than being served an empty list, so an outdated adapter fails loudly instead of appearing to work while delivering nothing. The polling adapter SHALL additionally declare the outbound contract version it speaks, and an absent or unsupported declaration SHALL fail with 400 naming what is expected.

Operations SHALL carry a stable id, the channel and conversation names, a kind (`ensure-topic`, `send`, or `close-topic`), and a kind-specific **structured** payload. `send` SHALL carry a typed message — one of `signal`, `answer`, `relay`, or `notice` — with markdown-valued free text and typed structured fields, plus the target thread id; it SHALL NOT carry pre-rendered display text. `ensure-topic` SHALL carry a topic descriptor (`conversation`, `pipeline?`, `source?`, `title`, `labels`, `kind`) rather than a rendered title string; `pipeline` is inferred and MAY be empty. `close-topic` SHALL carry the target thread id and asks the adapter to archive or close that thread on its transport. Escaping, length limits, chunking, truncation, and thread naming SHALL be the adapter's responsibility; the manager SHALL emit no transport markup and declare no maximum message size. Operations SHALL be derived from CR state or router actions such that an operation lost in flight (manager restart, adapter crash) is regenerated or safely skipped; delivery is at-least-once and adapters MUST tolerate duplicates by id. A `close-topic` operation SHALL be derivable from CR state for as long as it is outstanding, which the deleting conversation's finalizer guarantees.

#### Scenario: Adapter receives a topic-creation op
- **WHEN** a Conversation referencing a Channel with `adapter: slack` is reconciled with no `threadId` and an adapter is long-polling `/channel/ops?adapter=slack`
- **THEN** the adapter receives an `ensure-topic` operation identifying that conversation, carrying a descriptor it names the thread from

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

#### Scenario: Outdated outbound contract fails loudly
- **WHEN** an adapter that expects string-valued `send` ops polls for operations
- **THEN** the manager responds 400 naming the required contract version, rather than delivering messages it would post as empty text

#### Scenario: Send ops carry meaning, not markup
- **WHEN** a `send` op is delivered for an agent's answer
- **THEN** it carries an `answer` message with a markdown body and no transport markup, and the adapter renders it

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
The manager SHALL expose `POST /channel/inbound` accepting `{channel, threadId, text, sender?}` as the CONTINUATION path only: `threadId` is REQUIRED, and the message SHALL be routed through the transport-neutral router as a reply input on the matching conversation (busy-ack preserved). Resulting acks SHALL flow back to the adapter as `send` operations, and the message SHALL be relayed to the conversation's sibling channels as a `relay` message carrying its attribution as fields rather than composed into the text. In-process (registry) providers SHALL use the same operation pipeline so routing behavior is identical for built-in and external types.

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
- **THEN** it is relayed to the sibling channels as a `relay` message, attributed by each adapter's own rendering

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

### Requirement: ensure-topic may carry a previous thread id as a hint
The `ensure-topic` operation's topic descriptor MAY carry `previousThreadId`,
set only when a closed conversation is being reopened: the thread that
conversation used before its topics were archived.

It is a HINT, and IGNORING IT SHALL BE A VALID IMPLEMENTATION. An adapter whose
transport can un-archive SHOULD honour it and return that same thread id, so the
conversation continues where it left off with its history above the new
messages. An adapter whose transport has no such notion SHALL open a fresh
thread and return the new id. The manager SHALL record whatever comes back.

There SHALL be no `reopen-topic` operation kind. A new kind is a new thing every
adapter must implement, and most transports cannot un-archive — so most
implementations would be a second name for `ensure-topic`. The hint inverts the
cost: an adapter that does nothing with it is already correct.

Whether a transport can un-archive SHALL remain the ADAPTER's decision. The
manager holds no transport knowledge, by the same rule that keeps escaping,
message-length limits and thread naming out of it.

A consequence to accept: a reopened conversation may continue in its old thread
on one channel and a fresh thread on another. That is what those two transports
can actually do, and both are recorded.

#### Scenario: An adapter that can un-archive returns the same thread
- **WHEN** an adapter whose transport supports un-archiving receives `ensure-topic` with `previousThreadId`
- **THEN** it un-archives that thread and returns the same id

#### Scenario: An adapter that ignores the hint stays correct
- **WHEN** an adapter that cannot un-archive receives `ensure-topic` with `previousThreadId`
- **THEN** it creates a new thread, returns the new id, and the operation succeeds

#### Scenario: A fresh conversation carries no hint
- **WHEN** `ensure-topic` is delivered for a conversation that was never closed
- **THEN** `previousThreadId` is absent and the adapter creates a thread as it always has

### Requirement: Manager verbs let a bound surface reopen or delete a conversation
The manager SHALL expose two operations a channel adapter may call over its
existing authenticated contract: reopen and delete, each naming a conversation
and the channel the caller serves.

Reach SHALL be the BINDING — a surface may act on a conversation whose
`spec.channelRefs` names its channel — read from the CONVERSATION and never
taken from the request. Delete SHALL additionally refuse any conversation that
is not already `Closed`, with a reason naming the missing step.

These are not a remote CLOSE verb. Closing remains reachable only by posting
`/close` on a thread the surface holds. They exist because a closed conversation
holds no thread, so the proof of membership that closing relies on is
unavailable, and the binding that put the thread there is the next-strongest.

#### Scenario: A bound surface may reopen and delete
- **WHEN** an adapter calls either verb for a conversation whose bindings name its channel
- **THEN** the manager performs it

#### Scenario: An unbound surface is refused with a reason
- **WHEN** an adapter calls either verb for a conversation its channel is not bound to
- **THEN** the request is refused and the reason names the binding that would extend its reach

#### Scenario: Delete refuses anything not already closed
- **WHEN** an adapter asks to delete a conversation that is not `Closed`
- **THEN** the request is refused and the conversation is not closed as a side effect

### Requirement: A delete-conversation operation reports that a conversation ended for good
The manager SHALL deliver an operation of kind `delete-conversation` once per
bound thread when a `Conversation` is being deleted. It SHALL carry the target
thread id and a typed message in the ordinary markdown subset, stating that the
conversation and its record are gone and that a new message starts a new
conversation.

The operation reports a FACT about the conversation; what it means for a thread
SHALL be the adapter's decision. An adapter MAY post the message, archive the
thread, delete it, rename it, or express the ending in whatever way its
transport affords. It SHALL NOT silently do nothing: an adapter that cannot act
SHALL complete the operation with an error.

It is named for the CONVERSATION rather than the topic because the conversation
is what ended. `ensure-topic` and `close-topic` instruct an adapter about a
thread; this one informs it about a lifecycle event whose thread consequence the
adapter chooses.

`delete-conversation` SHALL REPLACE `close-topic` on the deletion path. A
conversation being deleted SHALL receive one or the other, never both, so an
adapter never has to decide whether a pair means one ending or two.

An adapter that has not implemented the kind SHALL remain correct: unknown kinds
are already tolerated, the operation is reported failed, and deletion proceeds
once the grace expires.

#### Scenario: A deleted conversation's threads are told
- **WHEN** a `Conversation` bound to two channels is deleted
- **THEN** each serving adapter receives one `delete-conversation` operation carrying that channel's thread id and the notice

#### Scenario: Closing and deleting are distinguishable
- **WHEN** an adapter receives `close-topic` and later `delete-conversation` for the same thread
- **THEN** the first means the thread is archived and the conversation may return, and the second means it will not

#### Scenario: A conversation deleted without being closed gets one operation
- **WHEN** a conversation that was never closed is deleted
- **THEN** its threads receive `delete-conversation` and no `close-topic`

#### Scenario: An adapter that does not implement the kind still deletes cleanly
- **WHEN** an adapter completes `delete-conversation` with an unknown-kind error
- **THEN** the deletion proceeds after the bounded grace and the object is removed

### Requirement: An adapter MAY report a thread read
The channel adapter contract SHALL offer `POST /channel/read`, by which an
adapter reports how far one or more of its threads have been seen. The request
SHALL name the channel and carry a list of `{threadId, readAt}` entries; the
manager SHALL resolve each thread to its conversation and write the watermark to
that conversation's status.

Each entry MAY additionally carry a `reader` — an OPAQUE key identifying who
read it, computed by the adapter. When present the manager SHALL record that
reader's own watermark; when absent it SHALL advance the channel-wide one. The
manager SHALL NOT derive, interpret or reverse the key, and an adapter SHALL NOT
send an address or any other identity in it.

The verb SHALL be OPTIONAL, and so SHALL the `reader` field within it. An adapter
that never calls it SHALL remain fully conformant, and its threads SHALL simply
carry no watermark; one that calls it without a reader SHALL remain fully
conformant, and its threads SHALL carry only the channel-wide mark.

#### Scenario: An adapter reports a read for one reader
- **WHEN** an adapter posts a read naming an opaque reader key
- **THEN** that reader's watermark advances and no other reader's does

#### Scenario: A reader-less report still works
- **WHEN** an adapter posts a read with no reader
- **THEN** the channel-wide watermark advances, exactly as before the field existed

The route SHALL be guarded by the same adapter authentication and channel scope
as every other `/channel/*` route: an adapter SHALL be able to report reads only
for channels it serves.

#### Scenario: An adapter reports a thread read
- **WHEN** an adapter posts a read for a thread on a channel it serves
- **THEN** the manager writes the watermark to that thread's binding and answers with the per-thread outcome

#### Scenario: An adapter cannot report for another adapter's channel
- **WHEN** an adapter scoped to one adapter name posts a read for a channel served by another
- **THEN** the request is refused with 403 and nothing is written

#### Scenario: An unauthenticated report is refused
- **WHEN** a read report arrives without a valid adapter token
- **THEN** it is refused with 401

#### Scenario: An adapter that never reports stays conformant
- **WHEN** an adapter implements every other contract operation and never calls this one
- **THEN** it serves its channels normally and its threads carry no watermark

### Requirement: A read report is a bounded batch with per-thread outcomes
A read report SHALL carry at most 50 entries, bounded by the manager and not only
by the caller. The response SHALL carry one outcome per requested thread —
`marked`, `skipped` or `failed` — with a reason for anything not marked, plus
totals. A batch in which some threads were not marked SHALL still be a successful
request, and a failure on one entry SHALL NOT stop the rest.

#### Scenario: An oversized batch is refused
- **WHEN** a read report carries more than 50 entries
- **THEN** the request is rejected with 400 and nothing is written

#### Scenario: A mixed batch succeeds with per-thread detail
- **WHEN** a batch marks some threads, skips others whose watermark would not advance, and fails on one whose conversation has been deleted
- **THEN** the response is 200 and names each thread with its own outcome and reason, plus the totals

#### Scenario: An unknown thread does not fail the batch
- **WHEN** a batch names a thread no conversation holds
- **THEN** that entry is reported failed with a reason and the remaining entries are still marked

### Requirement: Adapters absorb retryable backpressure inside the claim window
An adapter SHALL distinguish **retryable** transport conditions — rate limits,
timeouts, and transient server errors — from **terminal** ones, and SHALL retry
retryable conditions in-process before reporting the operation. Only a terminal
failure, or exhaustion of the adapter's retry budget, SHALL be reported as an
error to `POST /channel/ops/{id}/done`.

When the transport states how long to wait, the adapter SHALL honor that value
rather than applying its own backoff. The adapter's total in-process retry
budget for one operation SHALL remain strictly below the manager's claim
reclaim interval, so that an operation still being worked is never re-issued to
another claimant. An adapter that exceeds the reclaim interval converts the
manager's at-least-once redelivery into a duplicate the transport cannot
deduplicate, because the message has already been posted.

Reporting a retryable condition as an operation failure is permitted but
degrades the system to the manager's recovery path for a condition the adapter
could have ridden out; adapters SHALL NOT treat it as the normal path.

#### Scenario: Rate limit is slept out rather than reported
- **WHEN** the transport rejects a `send` with a retryable rate-limit error and a stated wait
- **THEN** the adapter waits the stated interval, retries the same operation, and reports success without the manager observing a failure

#### Scenario: Retry budget stays under the reclaim interval
- **WHEN** an operation's retries would exceed the manager's claim reclaim interval
- **THEN** the adapter abandons the retry, reports the operation as failed, and the manager re-derives it rather than two claimants posting the same message

#### Scenario: Terminal error is reported immediately
- **WHEN** the transport rejects an operation for a reason retrying cannot fix, such as a missing thread or a rejected credential
- **THEN** the adapter reports the failure without retrying and the manager applies its own recovery rules for that operation kind

### Requirement: Outbound pacing belongs to the adapter
An adapter SHALL pace its own outbound calls to its transport's documented
limits. The manager SHALL NOT model per-transport rate limits, expose a pacing
setting, or delay operation hand-out on a transport's behalf — a transport's
budget is transport knowledge, on the same footing as message length caps,
escaping, and thread naming, all of which the contract already places in the
adapter.

Pacing SHALL be applied to every call an operation makes, including thread
creation, so that a burst of new conversations is spread rather than rejected.

#### Scenario: Burst is spread rather than rejected
- **WHEN** the manager hands the adapter more operations in a short interval than the transport's budget allows
- **THEN** the adapter spreads the calls within budget and every operation completes, rather than a subset being rejected

#### Scenario: Manager declares no pacing
- **WHEN** an adapter for a transport with different limits is added
- **THEN** no manager-side change is required, because the manager holds no rate-limit knowledge for any transport
