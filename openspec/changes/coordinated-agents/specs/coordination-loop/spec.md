## Purpose
How a member's result reaches its coordinator, what a member may bind, and what bounds the loop.

## ADDED Requirements

### Requirement: A member's result becomes an input on its root

When a run completes on a conversation carrying `causedBy`, the manager SHALL
append the run's result as an input on the root conversation, attributed to the
member's entry name. This is the ONLY path by which a coordinator learns a
result; no adapter forwards and no channel is involved.

#### Scenario: Result routed
- **WHEN** a member conversation reports `/work/done`
- **THEN** the root has a new pending input carrying the result, and the root's next work unit includes it

#### Scenario: A closed root receives nothing
- **WHEN** a member reports a result after its root is `Closed`
- **THEN** no input is appended and the result stays on the member's own record

### Requirement: A caused conversation binds no human channel

A conversation created by a Coordinator SHALL bind no channel at creation. Its
inputs and results SHALL be delivered to no surface. The coordinator reaches
people through escalation on the root only.

#### Scenario: Member output stays off surfaces
- **WHEN** a member run completes
- **THEN** no send op is enqueued for any channel on the member's behalf

### Requirement: A conversation never receives its own output as input

`/channel/inbound` SHALL refuse an input whose origin surface is the target
conversation itself, and the manager SHALL never append a root's own result to
the root.

#### Scenario: Self-input refused
- **WHEN** an inbound message names a thread and carries an origin identifying the same conversation
- **THEN** it is refused with a reason, and no input is appended

### Requirement: Three limits bound a coordination, and exhaustion closes with a reason

A root SHALL track agents invoked, its own turns taken, and its age against the
snapshotted `maxAgents`, `maxTurns` and `deadline`. Past any of them the manager
SHALL close the root with `closeReason: budget-exceeded`, close every live
member with it, and run the escalation path so a person is told. A limit that
merely stopped work would be a silent drop.

#### Scenario: Fan-out limit
- **WHEN** a root has invoked `maxAgents` members and asks for one more
- **THEN** the invoke is refused naming the limit, and the root is closed `budget-exceeded` through escalation

#### Scenario: Deadline
- **WHEN** a root's age passes `deadline` with members still open
- **THEN** the root and every open member are closed `budget-exceeded`, and the root's escalation channels receive the closure

### Requirement: Invocation is asynchronous

Invoking an AgentCapability SHALL return the created (or reused) member conversation's
identity at once and SHALL NOT wait for a result. The invoke SHALL report
whether it created a conversation or attached to an existing one.

#### Scenario: Attach reported
- **WHEN** a coordinator invokes an AgentCapability with a signature a live member already carries
- **THEN** the invoke returns that member's name and reports `attached`
