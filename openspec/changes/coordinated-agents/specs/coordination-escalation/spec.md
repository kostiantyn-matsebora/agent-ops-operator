## Purpose
A coordinator opens a human thread by DECISION, not by arrival, and only the
tree's uncaused root ever opens one. The other outcomes are recorded closures
or a bubbled report to a parent.

## ADDED Requirements

### Requirement: Escalating the uncaused root binds its channels late, with a first message

An UNCAUSED conversation (no `causedBy`) SHALL bind the escalation channels
snapshotted onto it at creation (`spec.escalationChannelRefs`, copied from the
Coordinator's `channelRefs`) when the coordinating agent escalates and not
before.

Escalation SHALL read no Coordinator, so it works after the Coordinator is
edited or deleted. The thread on each channel SHALL open with the message the
agent supplied. Prior inputs and results SHALL NOT be replayed into it.

#### Scenario: Escalate opens a thread with the digest
- **WHEN** an uncaused conversation escalates with a message
- **THEN** each of the snapshotted channels gets a thread whose first post is that message
- **AND** no earlier member result is posted to it

#### Scenario: After escalation the root is an ordinary multi-channel conversation
- **WHEN** a person replies in an escalated thread
- **THEN** the reply is an input on that conversation, delivered to every other bound channel per the ordinary rule

### Requirement: Escalating a nested member bubbles instead of opening a thread

A conversation carrying `causedBy` SHALL open NO thread on `escalate`.

Instead the manager SHALL close it with the supplied message as its
`closeReason` and its result, which reaches its PARENT as an ordinary
member-result input (coordination-loop). It SHALL NOT bind
`spec.escalationChannelRefs` and SHALL NOT read a channel of any kind.

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
