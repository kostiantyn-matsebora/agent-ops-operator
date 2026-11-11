## MODIFIED Requirements

### Requirement: Escalating a nested member bubbles instead of opening a thread

A conversation carrying `causedBy` SHALL open NO thread on `escalate`.

Instead the manager SHALL close it with the supplied message as its
`closeReason` and its result. That result reaches its PARENT as an ordinary
member-result input (coordination-loop).

That SHALL be two tasks of one invocation on the tree's root key:

1. a `close` task with cascade, carrying the message as its reason
2. an `append-result` task appending the result to the parent

The second task runs only after the first settled. Its deduplication id SHALL
be derived from the task, never from the clock, so a redelivery appends the
result to the parent once.

A member binds no channel of its own — unchanged. It has nothing to post
into, and nothing for `escalate` to bind.

#### Scenario: A nested coordinator asks for help
- **WHEN** a member that is itself a Coordinator's root calls `escalate` with a message
- **THEN** that member closes with the message as its result, and no thread opens anywhere

#### Scenario: The bubble may repeat
- **WHEN** a parent receiving a bubbled escalation is itself nested and calls `escalate` again
- **THEN** the same closure-and-report happens one level further up, until a call reaches the uncaused root

#### Scenario: The report follows the close
- **WHEN** a member escalates
- **THEN** its `close` task settles first and the parent's `append-result` task runs after it on the same key

#### Scenario: A redelivered report appends once
- **WHEN** the `append-result` task is delivered twice
- **THEN** the parent holds one copy of the result

### Requirement: The uncaused root binds its channels at creation, and escalation posts into them

An UNCAUSED conversation (no `causedBy`) SHALL bind the Coordinator's
`channelRefs` at creation, unconditionally — the same way a Pipeline's
`channelRefs` bind to every conversation it creates. A human SHALL be able
to reply from the moment the conversation exists, whether or not the agent
ever escalates.

Escalating SHALL post the agent's message into those already-open threads.
It SHALL NOT bind anything, since there is nothing left to bind.

On the root that SHALL be one `escalate` task. It SHALL perform one patch
setting `status.escalatedAt` and the message. The patch SHALL be a no-op when
the conversation is already escalated.

Escalation SHALL read no Coordinator, so it works after the Coordinator is
edited or deleted. `status.escalatedAt` marks the moment it happened, for
display and for idempotency — a second `escalate` call on an
already-escalated root is a no-op.

Every input, before or after escalation, delivers to every bound channel by
the ordinary rule. Nothing about an input's timing relative to
`escalatedAt` withholds it.

#### Scenario: A reply before escalation is an ordinary input
- **WHEN** a person replies in an uncaused root's thread before the agent has escalated
- **THEN** the reply is delivered as an input, the same as any other bound conversation

#### Scenario: Escalate posts the digest into the open thread
- **WHEN** an uncaused root escalates with a message
- **THEN** that message posts into every one of its already-bound channels as the message
- **AND** no channel is newly bound by the call
- **AND** no earlier member result is replayed into it

#### Scenario: After escalation the root is an ordinary multi-channel conversation
- **WHEN** a person replies in an escalated thread
- **THEN** the reply is an input on that conversation, delivered to every other bound channel per the ordinary rule

#### Scenario: A member result reaches an already-open thread
- **WHEN** a member reports its result to an uncaused root before that root has ever escalated
- **THEN** the result is delivered to the root's bound channels the same way it would be after escalation

#### Scenario: A repeated escalate task is a no-op
- **WHEN** an `escalate` task is delivered to a root already carrying `escalatedAt`
- **THEN** it succeeds, changes nothing and posts nothing a second time
