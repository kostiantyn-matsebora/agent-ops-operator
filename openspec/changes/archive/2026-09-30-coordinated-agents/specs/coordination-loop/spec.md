## Purpose
How a member's result reaches its coordinator, what a member may bind, and what bounds the loop.

## ADDED Requirements

### Requirement: A member's result becomes an input on its parent

When a run completes on a conversation carrying `causedBy`, the manager SHALL
append the run's result as an input on the PARENT conversation — the one
`causedBy` names, one hop, never the tree's ultimate root.

Attribution names the member's entry name. This is the ONLY path by which a
coordinator learns a result. No adapter forwards and no channel is involved.

#### Scenario: Result routed
- **WHEN** a member conversation reports `/work/done`
- **THEN** the parent has a new pending input carrying the result, and the parent's next work unit includes it

#### Scenario: A closed parent receives nothing
- **WHEN** a member reports a result after its parent is `Closed`
- **THEN** no input is appended and the result stays on the member's own record

#### Scenario: A nested member's result stays at its own level
- **WHEN** a member that is itself a Coordinator's root reports a result
- **THEN** the result lands on that member's OWN parent, not on the tree's ultimate root

### Requirement: A caused conversation binds no human channel

A conversation created by a Coordinator SHALL bind no channel at creation. Its
inputs and results SHALL be delivered to no surface.

A coordinator reaches people only through escalation, which opens a thread
solely on the tree's UNCAUSED root — never on a nested member, however deep.

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

### Requirement: Three limits bound a coordination at every level

Every conversation that is itself a Coordinator's root — uncaused or nested —
SHALL track agents invoked, its own turns, and its age against its OWN
snapshotted `maxAgents`, `maxTurns` and `deadline`.

A nested level's limits are independent of its ancestors' and never pooled.

Past any limit the manager SHALL close that conversation with `closeReason:
budget-exceeded`, close every live member with it, and call `escalate`.

On the uncaused root this opens a human thread. On a nested member it
bubbles: the conversation closes, and the escalate message becomes an
ordinary input on its OWN parent. A limit that merely stopped work would be a
silent drop.

#### Scenario: Fan-out limit
- **WHEN** a conversation has invoked `maxAgents` members and asks for one more
- **THEN** the invoke is refused naming the limit, and that conversation is closed `budget-exceeded` through escalation

#### Scenario: Deadline
- **WHEN** a conversation's age passes `deadline` with members still open
- **THEN** it and every open member are closed `budget-exceeded`, and its own escalation path receives the closure

#### Scenario: A nested limit does not close the tree above it
- **WHEN** a nested member's own `maxTurns` is exhausted
- **THEN** only that member and its members close, and the closure reaches its parent as an input, not as a new human thread

### Requirement: Invocation is asynchronous

Invoking an AgentCapability SHALL return the created (or reused) member conversation's
identity at once and SHALL NOT wait for a result. The invoke SHALL report
whether it created a conversation or attached to an existing one.

#### Scenario: Attach reported
- **WHEN** a coordinator invokes an AgentCapability with a signature a live member already carries
- **THEN** the invoke returns that member's name and reports `attached`
