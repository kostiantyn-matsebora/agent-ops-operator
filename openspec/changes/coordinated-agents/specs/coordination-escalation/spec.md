## Purpose
A coordinator opens a human thread by DECISION, not by arrival, and the two other outcomes are recorded closures.

## ADDED Requirements

### Requirement: Escalation binds the coordinator's channels late, with a first message

A root conversation SHALL bind the escalation channels snapshotted onto it at
creation (`spec.escalationChannelRefs`, copied from the Coordinator's
`channelRefs`) when the coordinating agent escalates and not before. Escalation
SHALL read no Coordinator, so it works after the Coordinator is edited or
deleted. The thread on each SHALL open with
the message the agent supplied, and prior inputs and results SHALL NOT be
replayed into it.

#### Scenario: Escalate opens a thread with the digest
- **WHEN** a root escalates with a message
- **THEN** each of the Coordinator's channels gets a thread whose first post is that message
- **AND** no earlier member result is posted to it

#### Scenario: After escalation the root is an ordinary multi-channel conversation
- **WHEN** a person replies in an escalated thread
- **THEN** the reply is an input on the root, delivered to every other bound channel per the ordinary rule

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
