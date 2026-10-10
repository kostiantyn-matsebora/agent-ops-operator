# coordination-escalation

## Purpose
A coordinator opens a human thread by DECISION, not by arrival, and only the
tree's uncaused root ever opens one. The other outcomes are recorded closures
or a bubbled report to a parent.

## Requirements

### Requirement: Escalating a nested member bubbles instead of opening a thread

A conversation carrying `causedBy` SHALL open NO thread on `escalate`.

Instead the manager SHALL close it with the supplied message as its
`closeReason` and its result. That result reaches its PARENT as an ordinary
member-result input (coordination-loop).

A member binds no channel of its own — unchanged. It has nothing to post
into, and nothing for `escalate` to bind.

#### Scenario: A nested coordinator asks for help
- **WHEN** a member that is itself a Coordinator's root calls `escalate` with a message
- **THEN** that member closes with the message as its result, and no thread opens anywhere

#### Scenario: The bubble may repeat
- **WHEN** a parent receiving a bubbled escalation is itself nested and calls `escalate` again
- **THEN** the same closure-and-report happens one level further up, until a call reaches the uncaused root

### Requirement: Close and drop record why

Closing a root or a member without escalation SHALL stamp
`status.closeReason`, a short string the agent supplied, beside `closedAt`. A
close with no reason SHALL be refused from a coordinator.

#### Scenario: Solved without a person
- **WHEN** a coordinator closes its root with reason `resolved: restart cleared it`
- **THEN** the root is `Closed`, `closeReason` holds that text, and no thread was ever opened

### Requirement: Un-escalated incidents are never invisible

Every root closed without escalation SHALL remain listable with its reason and
its tree, so "why was I not told" has an answer.

#### Scenario: The record survives
- **WHEN** a root was closed `dropped: not relevant` an hour ago
- **THEN** it is listed with its reason, its members and their results intact

### Requirement: The uncaused root binds its channels at creation, and escalation posts into them

An UNCAUSED conversation (no `causedBy`) SHALL bind the Coordinator's
`channelRefs` at creation, unconditionally — the same way a Pipeline's
`channelRefs` bind to every conversation it creates. A human SHALL be able
to reply from the moment the conversation exists, whether or not the agent
ever escalates.

Escalating SHALL post the agent's message into those already-open threads.
It SHALL NOT bind anything, since there is nothing left to bind.

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
- **THEN** that message posts into every one of its already-bound channels as the digest
- **AND** no channel is newly bound by the call
- **AND** no earlier member result is replayed into it

#### Scenario: After escalation the root is an ordinary multi-channel conversation
- **WHEN** a person replies in an escalated thread
- **THEN** the reply is an input on that conversation, delivered to every other bound channel per the ordinary rule

#### Scenario: A member result reaches an already-open thread
- **WHEN** a member reports its result to an uncaused root before that root has ever escalated
- **THEN** the result is delivered to the root's bound channels the same way it would be after escalation
