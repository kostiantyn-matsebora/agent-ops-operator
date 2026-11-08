# coordination-loop

## Purpose
How a member's result reaches its coordinator, what a member may bind, and what bounds the loop.

## Requirements

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

The member's `conversationId` SHALL be minted by the invoke itself,
deterministically from the `agents[]` entry name, never from the signal
that opened the root — `conversation-provenance` records the field, this
requirement owns how it is minted.

A second invoke of the same entry attaches rather than duplicating for this
reason: the same id-equality rule every signal routes by.

#### Scenario: Attach reported
- **WHEN** a coordinator invokes the same `agents[]` entry a live member was already invoked as
- **THEN** the invoke returns that member's name and reports `attached`, because both invokes mint the same id

### Requirement: Every root turn ends in a move

A turn is one run on a conversation that carries `coordinatorRef`. A MOVE is
one of the coordination verbs the run called: `invoke`, `escalate` or
`close` on the aops server. Reads (`list_agents`, `read`, `get_tree`,
`list_conversations`, `get_conversation`, `list_open_roots`) are not moves.

A turn SHALL count as answered when any input the run consumed was a
person's message. A person asked, and the run's text is the answer.

An input is a person's message in two cases:

- it carries origin `channel`
- it carries any origin AND records a non-empty `surface` on its entry in
  `status.runs[].inputs[]`, the channel it was typed on, which a chat signal
  from a chat origination adapter does

An alert, a job tick and a posted task record no surface and are not.

When a run reports `succeeded`, made no move, and answered no person, the
manager SHALL treat the turn as a MISS and act on the conversation itself:

| Miss | The manager |
|---|---|
| first in a row | appends ONE corrective input, origin `manager`, naming the rule and the roster, and dispatches the next run as usual |
| second in a row | escalates the root with a digest saying the coordinating agent ended two turns without a move |

Two misses are IN A ROW when the second run's consumed inputs are exactly
the corrective input the first miss appended. Any other input between them
resets the count.

The manager SHALL never pick the move. It decides nothing about which agent,
whether to escalate, or whether to close. It only refuses to let a turn end
undecided.

A corrective turn is an ordinary turn. It spends `maxTurns` like any other,
and the budget edges stay as they are.

A run SHALL NOT be judged in any of these cases:

- it reports `failed`, and the existing failure paths apply
- it reports `succeeded` with continuity `unavailable`, which the runtime
  contract treats as a failure, or its input was held
- it is the run that reaches `maxTurns`, because the root closes on that edge
- the conversation is already `Closed`

A conversation carrying `causedBy` and no `coordinatorRef` SHALL NOT be
judged. A member answers its parent with its result, and that result is its
move.

#### Scenario: A verdict with no move is handed back
- **WHEN** a root's run consumes a member's result, writes a conclusion, calls no verb, and reports `succeeded`
- **THEN** the root has one new pending input of origin `manager` naming the rule, and the next run is dispatched with it

#### Scenario: Two misses escalate
- **WHEN** the run dispatched with that corrective input again calls no verb and reports `succeeded`
- **THEN** the root is escalated with a digest saying two turns ended without a move, and no further corrective input is appended

#### Scenario: Three invokes in one turn is one move
- **WHEN** a root's run calls `invoke` three times and reports `succeeded`
- **THEN** no corrective input is appended

#### Scenario: Answering a person is a move
- **WHEN** a root's run consumes an input of origin `channel`, writes an answer, calls no verb, and reports `succeeded`
- **THEN** no corrective input is appended

#### Scenario: Reads alone are not a move
- **WHEN** a root's run calls `list_agents` and `read`, calls no verb, answers no person, and reports `succeeded`
- **THEN** a corrective input is appended

#### Scenario: A chat-signal root is answered by prose
- **WHEN** a root opened by a person's message through a chat origination adapter runs, answers with prose, calls no verb, and reports `succeeded`
- **THEN** no corrective input is appended, because the input recorded the surface the person typed on

#### Scenario: A person's reply resets the count
- **WHEN** a first miss is followed by a person's reply, and the run answering that reply calls no verb
- **THEN** the run counts as answered, and the next miss after it is a first miss again, not a second

#### Scenario: A failed run is not judged
- **WHEN** a root's run reports `failed` having called no verb
- **THEN** no corrective input is appended and the failure is handled as any failed run is

#### Scenario: The run that reaches maxTurns is not judged
- **WHEN** a root's run makes no move, answers no person, and is the run that reaches `maxTurns`
- **THEN** no corrective input is appended and the root closes `budget-exceeded` as it does today

#### Scenario: An unavailable context is not judged
- **WHEN** a root's run reports `succeeded` with continuity `unavailable` and made no move
- **THEN** no corrective input is appended

#### Scenario: A held input is not judged
- **WHEN** a root's run reports `succeeded`, made no move, and its input was held because contexts are unreachable across the install
- **THEN** no corrective input is appended, and the input is dispatched again once contexts are reachable

#### Scenario: A closed root is not judged
- **WHEN** a root's run made no move and the root is already `Closed` when the run reports
- **THEN** no corrective input is appended and the root stays closed

#### Scenario: A member is not judged
- **WHEN** a member conversation's run calls no verb and reports `succeeded`
- **THEN** its result is routed to its parent as today, and no corrective input is appended to the member
