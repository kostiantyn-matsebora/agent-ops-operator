## ADDED Requirements

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
