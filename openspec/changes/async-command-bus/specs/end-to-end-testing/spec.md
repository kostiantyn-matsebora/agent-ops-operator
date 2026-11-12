## ADDED Requirements

### Requirement: The pack asserts the command bus's cluster facts
The system pack SHALL cover these facts about the command bus, each decided by
a cluster the conformance suite does not run:

1. **Multi-replica claim and complete.** A task claimed on one manager replica
   is completed through another, with the claim token the first replica was
   given. A channel message acknowledged on one replica is reported done on
   another.
2. **Restart durability.** A task accepted before every manager replica
   restarts, and before the bus server restarts, runs once to completion
   afterwards.
3. **Ordering under parallel consumers.** Tasks sharing an ordering key run one
   at a time in submission order while tasks of other keys run alongside, with
   more than one consumer replica claiming.
4. **Backoff, then dead letter.** A `conv` or `in.signal` task whose handler
   always fails retryably is retried with growing gaps and then goes dead. It
   appears in the dead-letter listing, and its id is resubmittable after the
   cool-off.
5. **A channel failure is final.** A channel message the adapter reports failed,
   or never reports before its delivery TTL, is not redelivered. It appears in
   the dead-letter listing, and reconciliation derives it again once its
   id is released.

Assertions SHALL read the task surface of the manager (`/state/tasks` and
`/state/dead-letters`), never the bus server directly, because no component
but the manager reaches the server.

#### Scenario: A claim crosses replicas
- **WHEN** an adapter reads and acknowledges a channel op on one replica and reports it done through another
- **THEN** the task is `succeeded` and no replica refused the completion

#### Scenario: A restart loses nothing accepted
- **WHEN** a command is accepted and every manager replica and the bus server are then restarted
- **THEN** the task completes once and its effect appears on the conversation exactly once

#### Scenario: Ordering holds with several consumers
- **WHEN** several `close` and `delete` tasks of one root and tasks of other roots are accepted together with more than one replica serving
- **THEN** the log shows the one root's tasks never overlapping and in submission order

#### Scenario: A failing channel message is not redelivered
- **WHEN** an adapter acknowledges a channel message and reports it failed
- **THEN** the bus never offers it again, the dead-letter listing carries it, and a later reconciliation delivers it once the surface recovers

#### Scenario: A failing task ends in a dead letter
- **WHEN** a task whose handler fails retryably on every attempt is accepted
- **THEN** its attempts are spaced further apart each time, it goes dead, and the dead-letter listing carries its code

### Requirement: The full tier drives the asynchronous flows end to end
The full tier SHALL drive these flows through the real HTTP surface of the
manager and the console, with the task stream observed throughout:

1. a signal opens a conversation, the conversation is closed, and it is then
   deleted
2. a bulk close of N conversations, observed as N independent closes, each with
   an invocation of its own, since the console fans a bulk close out as N
   ordinary `/close` messages
3. a cascade delete of a root with closed members, observed in member-first
   order
4. an inbound chat message delivered through the Telegram fake Bot API lane,
   and one through the console channel, each becoming one input
5. the console's stream dropped while entries are produced: after it
   reconnects every message arrived once, in order, and none needed a refresh
6. a manager restart mid-flight that loses no accepted command

Each flow SHALL assert on the task read surface and on the conversation
record. A flow SHALL NOT be satisfied by the reply code alone, because a
`202` means accepted.

#### Scenario: A conversation's whole life is observable
- **WHEN** a signal opens a conversation that is then closed and deleted
- **THEN** the task stream shows the ingest, close and delete tasks succeed in order, and the conversation is gone at the end

#### Scenario: A bulk close is N independent closes
- **WHEN** a console closes N conversations in one gesture
- **THEN** each close is accepted with its own invocation id, all succeed independently, and every conversation is `Closed`

#### Scenario: A cascade deletes members first
- **WHEN** a root with closed members is deleted
- **THEN** the log shows each member's `delete` task settle before the root's, and no conversation of the tree remains

#### Scenario: A Telegram message and a console message each become one input
- **WHEN** an inbound message arrives through the fake Bot API lane and another through the console channel
- **THEN** each is accepted, each becomes exactly one input of its conversation, and a repeated delivery of either adds no second input

#### Scenario: A dropped console stream loses nothing
- **WHEN** the console's stream drops while messages are produced and the console reconnects
- **THEN** every message the console never acknowledged is offered again, each arrives once in order, and no refresh is needed

#### Scenario: A restart mid-flight loses nothing
- **WHEN** the manager restarts while accepted tasks are queued or running
- **THEN** every accepted task reaches a terminal state and no command is lost or applied twice
