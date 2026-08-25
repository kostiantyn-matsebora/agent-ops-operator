## Purpose
The console renders a coordination as one incident: the root's turns interleaved with each member's start and finish.

## ADDED Requirements

### Requirement: The incident view is rooted at the uncaused conversation

A conversation without `causedBy` SHALL open as an incident view when it has
members: one timeline of its own inputs and runs, with each member's creation,
runs and closure interleaved by time, each member expandable to its own
transcript.

#### Scenario: A tree renders as one timeline
- **WHEN** a root has three members
- **THEN** the view shows the root's turns and the three members' starts, results and closures in time order

### Requirement: Members are reached from their root, and roots from their members

A member's transcript SHALL name its root and entry name and link to the
incident view; the conversation list SHALL group members under their root by
default, with a toggle to flatten.

#### Scenario: Navigation both ways
- **WHEN** a person opens a member conversation
- **THEN** they see which incident it belongs to and can open the incident view in one action

### Requirement: Un-escalated closures are shown, with their reason

Incidents closed without escalation SHALL appear in the list with their
`closeReason`, distinguishable from incidents a person was told about.

#### Scenario: The autosolved incident is visible
- **WHEN** a root was closed by its coordinator with a reason and no thread
- **THEN** the list shows it with that reason and a marker that nobody was notified
