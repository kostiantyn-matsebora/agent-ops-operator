# console-conversation-tree Specification

## Purpose
How a coordination reads in the conversations view: the tree in the list at any depth, the incident timeline in the uncaused root's thread, and what unread means for a member.

## Requirements

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

### Requirement: The composer follows channel binding, not escalation
The uncaused root's `channelRefs` bind at creation, the same way a
Pipeline's do (coordination-escalation). Its thread pane SHALL show the
composer from the moment it is open.

Escalating is a message the agent posts THROUGH that channel. It is never
what unlocks it.

The pane SHALL be read-only only in two cases, each naming itself:

- there is genuinely no bound channel (predates this behaviour, or the
  install's wiring declares none)
- the conversation is `Closed`

A member's pane SHALL be read-only at every depth, since a member never
binds a human channel.

#### Scenario: Open before escalating
- **WHEN** the operator opens a root whose coordinator has not escalated
- **THEN** the composer is shown and a reply is an ordinary input, delivered the same way it would be after escalation

#### Scenario: No channel at all
- **WHEN** the operator opens a root with an empty bound-channel set
- **THEN** no composer is shown and the pane says there is no channel to reply through

#### Scenario: A closed conversation is read-only
- **WHEN** the operator opens a root in phase `Closed` that still has bound channels
- **THEN** no composer is shown and the pane says the conversation is closed

### Requirement: A member names its place in the tree
A member's thread header SHALL show the chain from the uncaused root through
every parent to the member, each named by the entry it was invoked as. Each
step SHALL open that conversation.

The immediate parent SHALL be distinguishable from the rest of the chain.

#### Scenario: The chain is navigable
- **WHEN** the operator opens a member two levels deep
- **THEN** the header shows root, parent and member, and choosing the root opens the incident

### Requirement: Closing a root is shown as closing its members
The manager cascades a close through every live descendant. The confirmation for closing a selection SHALL count the selected ROOT conversations in its title and SHALL NOT add their descendants to that count.

Where the selected roots have descendants, the confirmation SHALL state in a separate line how many descendants close with them, at every depth, and how many of those are working.

A member SHALL NOT be selectable in this view and SHALL NOT be closable on its own.

The row menu SHALL NOT offer close for a member row.

The select-all control SHALL select root conversations only, and SHALL skip a root already closing or being deleted.

The server SHALL NOT trust the view. A close request that names a member directly SHALL report that name skipped, with a reason that it is a member and naming its parent, and SHALL NOT act on it.

#### Scenario: The confirmation counts members
- **WHEN** the operator closes a root with three members
- **THEN** the confirmation title says one conversation closes and a separate line says three members close with it

#### Scenario: Working members are named
- **WHEN** a selected root has three members and one is working
- **THEN** the member line says three members close with it and one is working

#### Scenario: A close request naming a member is skipped
- **WHEN** a close request names a member whose root is not named
- **THEN** that name is reported skipped with a reason naming its parent, and every other name is processed normally

#### Scenario: A member cannot be closed directly
- **WHEN** the list shows a root and its members
- **THEN** the members carry no selection control, select-all selects the root only, and no close request names a member

### Requirement: Deleting a root is shown as deleting its members
The manager cascades a delete through every already-closed descendant, the way it cascades a close through every live one.

The confirmation for deleting a selection SHALL count the selected ROOT conversations in its title and SHALL NOT add their descendants to that count.

Where the selected roots have descendants, the confirmation SHALL state in a separate line how many descendants delete with them, at every depth.

A member SHALL NOT be deletable on its own from this view, on the same
grounds as closing it. The row menu SHALL NOT offer delete for a member row,
and a member SHALL NOT be selectable.

The select-all control SHALL follow the same rule as for closing a root.

The server SHALL NOT trust the view. A delete request that names a member directly SHALL report that name skipped, with a reason that it is a member and naming its parent, and SHALL NOT act on it.

#### Scenario: The confirmation counts members
- **WHEN** the operator deletes a root with three members
- **THEN** the confirmation title says one conversation deletes and a separate line says three members delete with it

#### Scenario: A delete request naming a member is skipped
- **WHEN** a delete request names a closed member whose root is not named
- **THEN** that name is reported skipped with a reason naming its parent, and every other name is processed normally

#### Scenario: A member cannot be deleted directly
- **WHEN** the list shows a closed root and its closed members
- **THEN** the members carry no selection control, select-all selects the root only, and no delete request names a member

#### Scenario: The cascade reaches unselected members
- **WHEN** the operator deletes a closed root
- **THEN** its cascade reaches every closed descendant although none was selected

### Requirement: An incident nobody was told about is visible
A root closed by its coordinator without escalation SHALL appear in the
list with its close reason and a marker that no person was notified,
distinct from a root that escalated.

#### Scenario: The autosolved incident
- **WHEN** a coordinator closed its root with a reason and never escalated
- **THEN** the row shows the reason and the marker that nobody was notified
