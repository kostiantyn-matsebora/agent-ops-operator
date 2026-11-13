## Purpose
How the manager uses the command bus. A request is validated and durably
accepted at once, then performed later by a consumer on any replica, and what
became of it is a task a caller can read.

This capability owns the command inventory, the accept and process split, the
error codes a handler reports, the task read surface and the list of
operations that stay synchronous.

## ADDED Requirements

### Requirement: Every asynchronous request is accepted, then processed
A request for an asynchronous command SHALL run an ACCEPT step in the request
and a PROCESS step on a manager consumer. The consumer MAY be on any replica.

The accept step SHALL be synchronous and cheap. It SHALL do, in order:

1. authenticate the caller and check its adapter or state scope
2. check the shape of the body
3. refuse an unknown channel or source
4. for a signal, refuse for the `Wired=False` and attribute-filter reasons
5. for a signal, record the activity "signal received" receipt
6. for a coordinator verb, stamp the move
7. mint an invocation id and durably enqueue the tasks
8. reply `202` with the invocation id and the task ids, except that a coordinator verb keeps the `200` and body it returns today and gains the same two fields

Everything else SHALL be the process step. The accept step SHALL NOT perform
the command's effect.

#### Scenario: The reply carries the invocation and its tasks
- **WHEN** a console asks to close one conversation
- **THEN** the reply is `202` and its body names the invocation and the task, as in the example below

```json
{
  "invocationId": "inv-4c2a",
  "taskIds": ["tsk-91e0"]
}
```

#### Scenario: A refusal at accept enqueues nothing
- **WHEN** a request fails authentication, body shape, or names an unknown channel
- **THEN** it is refused with its reason and no task is recorded

#### Scenario: The effect happens in the process step
- **WHEN** a close is accepted
- **THEN** the conversation has not necessarily reached `Closed` when the reply is sent, and the task shows when it has

### Requirement: Each command has one queue and one ordering key
Every asynchronous command SHALL map to one queue and one ordering key:

| Command | Queue | Ordering key |
|---|---|---|
| `ingest-chat` | `conv` | the root conversation of the thread's conversation |
| `close` | `conv` | the root conversation of the target |
| `exit` | `conv` | the root conversation of the target |
| `reopen` | `conv` | the root conversation of the target |
| `delete` | `conv` | the root conversation of the target |
| `reset-context` | `conv` | the root conversation of the target |
| `escalate` | `conv` | the root conversation of the target |
| `ingest-signal` | `in.signal` | the source and the `initiationId` |
| `ensure-topic`, `send`, `close-topic`, `delete-conversation` | `out.<channel>` | the conversation |

What the commands cover:

- `ingest-chat` covers an inbound chat message. A console send SHALL go through it.
- The typed commands `/close`, `/exit` and `/reopen` arrive as `ingest-chat`, and the processor turns them into the tasks above.
- `ingest-signal` covers every signal adapter, a console start (a chat signal) and chat origination.
- `send` covers an input card, a run reply, a notice, a farewell, a reopen notice and an escalation message.

#### Scenario: Two closes under one root are serialized
- **WHEN** a close for a member and a delete for its root are accepted together
- **THEN** both tasks carry the root's key and run one after the other in submission order

#### Scenario: Two unrelated conversations run in parallel
- **WHEN** closes for conversations of two different roots are accepted together
- **THEN** their tasks may run at the same time

### Requirement: A task is atomic and a batch is only a shared invocation
A task SHALL carry one verb and one target. A request that acts on several
conversations SHALL enqueue one task per conversation and give every task the
same invocation id. The invocation id SHALL be the only thing a batch is.

A cascade SHALL enqueue one task per member, members before the parent, all on
the ROOT conversation's key. The tree SHALL be read at accept time, which is a
cheap read.

A queue SHALL cap how many tasks of one invocation run at once, so a large
batch cannot flood a lane.

#### Scenario: A bulk close is N independent closes
- **WHEN** a console closes fifty conversations in one gesture
- **THEN** it sends fifty ordinary closes, each is accepted with its own invocation id and task, and each task closes one conversation

#### Scenario: A cascade is ordered
- **WHEN** a root with two members is deleted
- **THEN** one task per member is enqueued before the root's task, and the root's task is claimable only after both settled

#### Scenario: A batch cannot flood a lane
- **WHEN** an invocation holds more tasks than its queue allows in flight
- **THEN** no more than the cap run at once and the rest wait

### Requirement: An inbound chat message resolves its key at accept
The accept step of an inbound chat message SHALL resolve the message's thread
to its conversation and so to its ordering key. A thread that resolves to no
conversation SHALL be refused or reported unwired exactly as before. No task
SHALL be enqueued for it.

#### Scenario: A reply into a known thread is keyed by its root
- **WHEN** a message arrives in the thread of a member conversation
- **THEN** its task carries the key of that member's root conversation

#### Scenario: A message in an unknown thread is not queued
- **WHEN** a message arrives in a thread no conversation owns
- **THEN** it is refused or reported unwired as today and no task exists for it

### Requirement: Inbound commands are idempotent by a key the caller can repeat
An inbound signal SHALL keep its `initiationId` as its deduplication key. An
inbound chat message SHALL use an `Idempotency-Key` header when the caller
sends one. Absent the header, the key SHALL be derived from the channel, the
thread, the text and the sender.

A command repeated under the same key SHALL return the first command's
invocation and task ids and SHALL NOT create a second task.

#### Scenario: A redelivered chat message is processed once
- **WHEN** an adapter posts one inbound message twice with the same `Idempotency-Key`
- **THEN** both replies carry the same invocation id and the message becomes one input

#### Scenario: A message without a header is still deduplicated
- **WHEN** an adapter posts the same text from the same sender in the same thread twice without the header
- **THEN** the second post returns the first one's invocation

### Requirement: Escalation is decomposed by where the conversation sits
An `escalate` command SHALL be ONE `escalate` task on the escalating conversation's key.
Its command id SHALL derive from the conversation, never from the clock.

| Conversation | The task does |
|---|---|
| root, uncaused | one patch setting `status.escalatedAt` and the message, a no-op when already escalated |
| member, caused | closes the member with the message as its reason, and appends the result to the parent |

Each step SHALL be conditional on the state it finds, so a redelivery appends the
result to the parent once.

#### Scenario: A redelivered root escalation is a no-op
- **WHEN** an `escalate` task is delivered twice to an uncaused root
- **THEN** the second delivery changes nothing and succeeds

#### Scenario: A member escalation closes and reports
- **WHEN** a member calls `escalate` with a message
- **THEN** one `escalate` task is enqueued on the member's key, and it closes the member and appends the message to the parent

#### Scenario: A redelivered member escalation reports once
- **WHEN** the `escalate` task's lease lapses after the parent was appended and it is delivered again
- **THEN** the parent holds one copy of the result

### Requirement: Channel operations run on the bus as hand-over queues
Outbound channel operations SHALL be tasks on the per-channel `out.<channel>` queues, delivered under the three-step contract of the channel adapter: read, acknowledge, then report done or failed.

A queue of this family SHALL be a hand-over queue: the bus does not retry a message, because the adapter retries against its own surface until the message's delivery TTL.

- a derived, stable operation id SHALL deduplicate against a pending task and a succeeded one
- a message read and not acknowledged SHALL be offered again, and one acknowledged SHALL never be sent by the manager again
- the delivery TTL SHALL start at the acknowledgement and SHALL be declared per message kind: stable 10 minutes, ephemeral 2 minutes, topic operations 5 minutes, `delete-conversation` 5 minutes
- a message with no report when its delivery TTL passes SHALL be failed by the manager, and a failure, reported or by timeout, SHALL be final and never redelivered
- after a task fails its id SHALL be parked for the kind's cool-off, then released, so reconciliation MAY derive it again
- a fire-and-forget `send` SHALL also carry a short time to live while it is still unread
- `delete-conversation` SHALL never be parked, so a deletion never wedges on a down adapter
- the chart value `claimStalenessSeconds` SHALL be inert

The queue depth read at `/api/queues` SHALL keep working from the per-queue statistics.

The `done` call of `/channel/ops/{id}/done` SHALL run its completion handlers
synchronously before it replies, because a thread binding is what inbound
messages resolve through:

- the thread binding on `ensure-topic`
- the delivered and undelivered markers
- the archived marker

The adapters named by `CHANNEL_RECEIPT_ADAPTERS` (the console by default) are the declared exception. For them the acknowledgement is only the receipt of the message.

It SHALL carry no delivery TTL and no report, and the manager SHALL run the handlers and delete the message at the acknowledgement. No adapter's name SHALL appear in manager or bus code.

#### Scenario: A stale notice is not delivered after a restart
- **WHEN** a fire-and-forget `send` outlives its time to live unread while the manager was down
- **THEN** it is expired and no adapter receives it

#### Scenario: claimStalenessSeconds sets nothing
- **WHEN** an install sets the chart value `claimStalenessSeconds` to a non-default number
- **THEN** the chart still renders and no message's delivery changes

#### Scenario: A deletion is never wedged
- **WHEN** no adapter acknowledges a `delete-conversation` task within its time to live
- **THEN** it expires, the finalizer releases, and the task is never parked

#### Scenario: A failed send is final
- **WHEN** an adapter acknowledges a `send` and reports it failed
- **THEN** the bus does not retry it, its id is parked, and no second delivery of it happens

#### Scenario: A dead send is derived again after its cool-off
- **WHEN** a `send` task failed and its park cool-off has passed while the reply is still owed
- **THEN** reconciliation derives the same id again and a new task runs

#### Scenario: A thread binding exists when the done call replies
- **WHEN** an adapter reports an `ensure-topic` task done with its thread id and the reply to that call returns
- **THEN** the conversation already records the thread and an inbound message in it resolves

#### Scenario: A made-up adapter needs no manager code
- **WHEN** an adapter whose name no code mentions reads, acknowledges and reports a message
- **THEN** the full contract works with the generic delivery TTL of the message's kind

#### Scenario: The console is receipt-only
- **WHEN** the console acknowledges a message
- **THEN** the reply says it was consumed with no delivery TTL, the message is deleted, and the console reports nothing

### Requirement: A reset-context is a task
A request to reset a conversation's context SHALL be accepted and enqueued as
one `reset-context` task on the conversation's root key. The task SHALL clear
the runtime context handle, the retired session id and the context checkpoint,
and SHALL record that the continuity condition changed.

A conversation that carries no context handle SHALL make the task succeed and
report that there was nothing to clear. It SHALL NOT go dead.

#### Scenario: A reset is accepted before it is done
- **WHEN** a surface asks to reset the context of a conversation that carries a handle
- **THEN** the reply is `202` with the invocation and task ids, and the handle is cleared when the task succeeds

#### Scenario: Nothing to reset
- **WHEN** a `reset-context` task runs on a conversation with no context handle
- **THEN** the task succeeds, reports that nothing was cleared, and no error is recorded

### Requirement: A reply to a command means accepted
A `202` from `close`, `exit`, `reopen`, `delete`, `reset-context` or
`escalate` SHALL mean the command was accepted. So SHALL the `200` that the
coordinator verbs `close` and `escalate` already return. It SHALL NOT mean it was
performed. A caller that needs the outcome SHALL read the task.

A refusal the caller could act on that depends only on cheap reads SHALL still
be returned at accept. A request to delete a conversation that is not `Closed`
SHALL be refused with `409` at accept, naming the missing step.

#### Scenario: Delete of a live conversation is refused at once
- **WHEN** a surface asks to delete a conversation that is `Working`
- **THEN** the reply is `409` naming that it must be closed first, and no task is enqueued

#### Scenario: A delete is accepted before it is done
- **WHEN** a surface asks to delete a closed conversation
- **THEN** the reply is `202` and the conversation may still exist until the task succeeds

### Requirement: A handler reports its outcome with a closed code
A handler that does not succeed SHALL report one code of the bus's closed
vocabulary. The codes mean:

| Code | Means | Example |
|---|---|---|
| `not_found` | the target is gone | a conversation deleted before its close ran |
| `precondition_failed` | the target's state forbids the command now | `/exit` while a run is inflight |
| `forbidden` | the actor's reach does not cover the target | a surface naming a binding it lacks |
| `conflict` | a conditional write lost a race | a stale revision |
| `unavailable` | a dependency could not be reached | the state store or the cluster API |
| `invalid_command` | the command cannot be performed as stated | a body the process step cannot read |
| `internal` | anything else | a defect |

#### Scenario: A gone target is not retried
- **WHEN** a close task finds its conversation already deleted
- **THEN** it reports `not_found` and goes dead without a retry

#### Scenario: A lost race is retried
- **WHEN** a handler's conditional write is refused as stale
- **THEN** it reports `conflict` and the bus retries it

#### Scenario: A dead command leaves the record true
- **WHEN** a close task goes dead
- **THEN** the conversation record is unchanged and still tells the truth, and the task and its dead letter tell what failed

### Requirement: These operations stay synchronous
The following SHALL NOT become asynchronous, for the reasons given:

| Operation | Why it stays synchronous |
|---|---|
| reads, streams and sessions | they return state and have no effect to defer |
| `invoke` | its reply carries the member and the budget reservation, its refusals must be immediate, and an accepted-then-failed `invoke` would still count as a move |
| the coordinator reads | they return state |
| the runtime's `/work`, `/work/done` and `/work/context` | the move-per-turn judgement and the corrective input are inside `/work/done` and need an immediate answer |
| the consumer ends of `/channel/ops`, `/channel/ops/{id}/ack`, `/channel/ops/{id}/done` and `/channel/ops/stream` | the read, the hand-over and its report are the bus's consumer |
| adapter state and status | small writes the caller reads back |
| `/activity` | telemetry with no durable effect |
| mark read and mark unread | monotonic and cheap |
| the accept step | it is what makes the rest durable |

#### Scenario: Invoke refuses at once
- **WHEN** a Coordinator's agent calls `invoke` and its budget is spent
- **THEN** the call is refused in the same reply and no member exists

#### Scenario: The runtime protocol is unchanged
- **WHEN** a runtime reports a run through `/work/done`
- **THEN** the manager answers within the call, including any corrective input, and no task is enqueued for it

### Requirement: Tasks are readable through the state surface
The manager SHALL serve these read-only endpoints on the existing `/state/*`
token scheme:

| Endpoint | Returns |
|---|---|
| `GET /state/tasks` | per-queue statistics: depth, in-flight and the oldest waiting age for each queue and kind |
| `GET /state/tasks/{id}` | one task by its task uid |
| `GET /state/tasks/stream` | a server-sent stream of task changes |
| `GET /state/dead-letters` | the dead-letter records |

A task SHALL carry its command id, invocation id, queue, kind, ordering key, state, attempt, code and detail, and its timestamps. It SHALL carry no order or sequence field: the order of tasks is the log's own.

The stream frames SHALL carry the log position as the frame id, and the stream SHALL be resumable through it with `Last-Event-ID`.

```json
{
  "uid": "7f3c1a9e-0b2d-4e61-9a55-3c0f6d2e8b14",
  "commandId": "close:swift-otter",
  "queue": "conv",
  "key": "swift-otter",
  "invocationId": "inv-4c2a",
  "state": "retrying",
  "attempt": 2,
  "maxAttempts": 5,
  "code": "unavailable",
  "detail": "state store unreachable",
  "notBefore": "2026-11-11T09:00:12Z",
  "submittedAt": "2026-11-11T09:00:00Z",
  "updatedAt": "2026-11-11T09:00:07Z"
}
```

There SHALL be no endpoint that retries a task or dismisses a dead letter. A
non-leader replica SHALL serve these reads itself and SHALL NOT proxy them.

#### Scenario: A caller reads what became of its command
- **WHEN** a caller reads a task by the id it was given
- **THEN** it sees the task's state, attempt and, if it failed, its code and message

#### Scenario: A stream resumes without a gap
- **WHEN** a stream client reconnects with `Last-Event-ID`
- **THEN** it receives every change after that point in revision order

#### Scenario: A dead letter is read but not acted on
- **WHEN** a caller lists dead letters
- **THEN** it sees each failed task and finds no endpoint to retry or dismiss one

#### Scenario: The scope is the existing one
- **WHEN** a caller without a `/state/*` token asks for a task
- **THEN** it is refused as any other `/state/*` read is

### Requirement: The runtime contract is unchanged and the channel adapter contract moves once
The runtime contract SHALL NOT change. A reply that gains the invocation and
task ids SHALL gain them additively and keep every field it had.

The channel adapter contract SHALL change in one place only, the delivery of an outbound operation: read, acknowledge, then report.

That change is BREAKING for a channel adapter and SHALL be published as a new major version of `docs/contracts/channel-adapter.md`, with the upgrade note that adapter images move together with the manager.

Every other call of that contract SHALL be unchanged.

#### Scenario: An existing runtime keeps working
- **WHEN** a runtime that predates the bus reports a run through `/work/done`
- **THEN** it reads the reply as before

#### Scenario: A signal adapter keeps working
- **WHEN** an adapter that predates the bus posts an inbound signal
- **THEN** it reads the reply as before and ignores any added fields

#### Scenario: A channel adapter must acknowledge
- **WHEN** a channel adapter built against the previous contract reads an operation and never acknowledges it
- **THEN** the manager offers the same operation again on its next read, which is why the adapter is upgraded with the manager

### Requirement: Three things are not part of this capability
This capability SHALL NOT define a console view of tasks. It SHALL NOT define a
retry or dismiss action on a dead letter. It SHALL NOT make `invoke`
asynchronous.

#### Scenario: No task action is offered
- **WHEN** the task read surface is inspected
- **THEN** every endpoint is a read
