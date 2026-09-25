## Purpose
The console renders a coordination as one incident: the uncaused root's turns
interleaved with every descendant's start and finish, at any nesting depth.

## ADDED Requirements

### Requirement: The incident view is rooted at the uncaused conversation, and nests

A conversation without `causedBy` SHALL open as an incident view when it has
members: one timeline of its own inputs and runs, with each direct member's
creation, runs and closure interleaved by time.

A member that is itself a Coordinator's root SHALL expand to its OWN nested
timeline in place, rather than only linking out — so the whole tree is
readable from the uncaused root without leaving the view.

#### Scenario: A tree renders as one timeline
- **WHEN** a root has three members
- **THEN** the view shows the root's turns and the three members' starts, results and closures in time order

#### Scenario: A nested coordination expands in place
- **WHEN** one of the root's members is itself a Coordinator's root with its own members
- **THEN** expanding that member reveals its own timeline nested inside the root's

### Requirement: A conversation is reached from its ancestors, and its ancestors from it

A conversation's transcript SHALL name its PARENT and entry name and link to
the incident view. The conversation list SHALL group every descendant under
its uncaused root by default, at whatever depth, with a toggle to flatten.

#### Scenario: Navigation both ways
- **WHEN** a person opens a member conversation
- **THEN** they see which incident it belongs to and can open the incident view in one action

#### Scenario: A nested member names its own parent, not the ultimate root
- **WHEN** a person opens a conversation nested two levels deep
- **THEN** its transcript names its immediate parent, and the incident view shows its place in the whole tree

### Requirement: Un-escalated closures are shown, with their reason

Incidents closed without escalation SHALL appear in the list with their
`closeReason`, distinguishable from incidents a person was told about.

#### Scenario: The autosolved incident is visible
- **WHEN** a root was closed by its coordinator with a reason and no thread
- **THEN** the list shows it with that reason and a marker that nobody was notified
