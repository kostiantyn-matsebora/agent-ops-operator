## Purpose

How a coordination reads in the conversations view: the tree in the list at any depth, the incident timeline in the uncaused root's thread, and what unread means for a member.

## ADDED Requirements

### Requirement: The list groups a coordination under its uncaused root
Conversations caused by another SHALL be listed under their uncaused root
by default, indented by depth, and a control SHALL flatten the list. Depth
SHALL be the length of the chain of parents, followed one hop at a time.

Every conversation that is a coordinator's root at its own level SHALL show,
from its own budget:

- a caret that collapses its members
- the member count
- the current turn against the turn limit
- the time left against the deadline, where one is set

A conversation whose parent is not in view SHALL sit at the top level with
a marker that its parent is missing, never be dropped.

#### Scenario: Members sit under their root
- **WHEN** a root has three members and grouping is on
- **THEN** the three rows sit indented beneath the root, and collapsing the caret hides them

#### Scenario: A coordinating member nests its own
- **WHEN** a member is itself a coordinator's root and caused two further conversations
- **THEN** those two sit one level deeper than the member, and the member's row shows its own turn and deadline

### Requirement: A member is never unread on its own
A member conversation has no console thread, so it SHALL NOT carry an
unread count. Its result reaches the root as an input and SHALL be counted
on the root by the console-unread rule.

#### Scenario: A member's result counts on the root
- **WHEN** a member reports done and the root has not been read since
- **THEN** the root's unread count rises by one and the member's stays absent

### Requirement: The uncaused root's thread is the incident timeline
Opening the uncaused root SHALL show one column in time order, holding:

- the root's own inputs and answers
- each member's invocation, as a line naming the entry it was invoked as
- each member's result, as a card naming the member with a link to its transcript
- a divider marking escalation, where one happened
- every message after it, as ordinary chat

A coordinating member's invocations and results SHALL nest inside its card,
collapsed by default, so the whole tree is readable without leaving the
view.

A nested coordinator's escalation reaches its parent as a result and SHALL
render as that member's result card, marked as an escalation.

#### Scenario: One timeline for the whole incident
- **WHEN** a root with two members is opened
- **THEN** the coordinator's turns, both invocations, both results and the escalation divider read in one column in time order

#### Scenario: A nested escalation is a result, not a divider
- **WHEN** a member that is itself a coordinator's root escalates
- **THEN** its parent's timeline shows a result card marked as an escalation, and no divider appears below the uncaused root's

### Requirement: The composer follows escalation
Before the uncaused root has escalated it is bound to no channel, so its
thread pane SHALL be read-only and SHALL say why. After escalation it SHALL
behave as any multi-channel conversation.

A member's pane SHALL be read-only at every depth, since a member never
binds a human channel.

#### Scenario: Not yet escalated
- **WHEN** the operator opens a root whose coordinator has not escalated
- **THEN** no composer is shown and the pane says the coordinator has not asked for a person

### Requirement: A member names its place in the tree
A member's thread header SHALL show the chain from the uncaused root through
every parent to the member, each named by the entry it was invoked as. Each
step SHALL open that conversation.

The immediate parent SHALL be distinguishable from the rest of the chain.

#### Scenario: The chain is navigable
- **WHEN** the operator opens a member two levels deep
- **THEN** the header shows root, parent and member, and choosing the root opens the incident

### Requirement: Closing a root is shown as closing its members
The confirmation for closing a selection containing a coordinator's root
SHALL state how many descendants close with it, at every depth. A member
SHALL NOT be closable on its own from this view, and the refusal SHALL name
its parent.

#### Scenario: The confirmation counts members
- **WHEN** the operator closes a root with three members
- **THEN** the confirmation says four conversations close

### Requirement: An incident nobody was told about is visible
A root closed by its coordinator without escalation SHALL appear in the
list with its close reason and a marker that no person was notified,
distinct from a root that escalated.

#### Scenario: The autosolved incident
- **WHEN** a coordinator closed its root with a reason and never escalated
- **THEN** the row shows the reason and the marker that nobody was notified
