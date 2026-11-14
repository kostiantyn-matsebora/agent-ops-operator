## ADDED Requirements

### Requirement: A conversation read carries whether it is saved
`/state/conversations` and `/state/conversations/{name}` SHALL carry `savedAt` when
the conversation is Saved and omit it otherwise, under every backend.

#### Scenario: A saved conversation reports when
- **WHEN** a reader lists conversations and one is Saved
- **THEN** that entry carries `savedAt` and the others do not

#### Scenario: The feed carries a change
- **WHEN** a conversation is saved or unsaved
- **THEN** the resumable feed emits the changed conversation like any other update
