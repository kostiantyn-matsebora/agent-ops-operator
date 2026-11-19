## MODIFIED Requirements

### Requirement: The inbox column lists scopes, each with its count
The inbox column SHALL list, in this order: the filters Open, Working, Mine
and Errored, then every Ready Pipeline and Coordinator, then Closed. Open
SHALL be the scope the view lands on.

Open SHALL list root conversations whose phase is not `Closed`. Closed SHALL
list root conversations whose phase is `Closed`. The Closed scope SHALL be
labelled Closed everywhere it is named, and never Archive.

Each scope SHALL show its count where one is non-zero. The count SHALL be the
number of unread root conversations in that scope, computed by the
console-unread rule. A member conversation SHALL NOT be counted.

Mine SHALL mean conversations the viewer's own identity started or replied
in.

#### Scenario: The view lands on Open
- **WHEN** the operator opens the Conversations view with no scope chosen
- **THEN** the Open scope is selected and lists only conversations that are not closed

#### Scenario: A pipeline scope narrows the list
- **WHEN** the operator selects a Pipeline in the inbox
- **THEN** the list shows only open conversations attributed to that Pipeline, and the scope's count matches its unread rows

#### Scenario: Open counts unread roots
- **WHEN** three open root conversations are unread and one open root is read
- **THEN** the Open badge reads three

#### Scenario: Closed counts unread roots
- **WHEN** two closed root conversations hold unread messages and four closed roots are read
- **THEN** the Closed badge reads two

#### Scenario: Counts are unread messages, not changed rows
- **WHEN** a run is dispatched on a conversation the viewer has read
- **THEN** no scope's count changes

## ADDED Requirements

### Requirement: Closed conversations are reached through Closed only
The Conversations view SHALL NOT offer a control that includes closed conversations in any scope but Closed. Open, Working, Mine, Errored and every Pipeline and Coordinator scope SHALL exclude closed conversations always. Closed SHALL show closed conversations only.

The view SHALL NOT read or write a "show closed" preference, and a value left by an earlier version SHALL be ignored.

#### Scenario: No scope offers a closed toggle
- **WHEN** the operator selects any scope
- **THEN** the list header carries no "Show closed" control

#### Scenario: A closed conversation is not in Working
- **WHEN** a conversation is closed while its last run was Working
- **THEN** it appears under Closed and under no other scope

#### Scenario: A stored preference is ignored
- **WHEN** the browser holds a "show closed" value from an earlier version
- **THEN** the view behaves as if it were absent and every scope but Closed excludes closed conversations
