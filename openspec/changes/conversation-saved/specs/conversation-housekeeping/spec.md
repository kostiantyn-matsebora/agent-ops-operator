## ADDED Requirements

### Requirement: A saved conversation is never reclaimed automatically
Automatic closing and automatic deleting SHALL skip a Saved conversation, and a
member of a Saved root, whatever the idle or closed age. The check SHALL be made at
the moment of the decision, not when the work was scheduled. Clearing the mark
SHALL make the conversation eligible again, measured from its existing origin
(last activity, or `closedAt`).

#### Scenario: Idle and saved is not autoclosed
- **WHEN** autoclose is enabled and a Saved conversation passes its idle window
- **THEN** it stays Idle

#### Scenario: Closed and saved is not autodeleted
- **WHEN** autodelete is enabled and a Saved, Closed conversation passes its closed window
- **THEN** it is not deleted

#### Scenario: Saved between scheduling and running
- **WHEN** a conversation is saved after an autoclose task was queued and before it runs
- **THEN** the task leaves it open

#### Scenario: Unsaved resumes the clock
- **WHEN** a Saved conversation that has long passed the delete window is unsaved
- **THEN** it becomes eligible at once, not a full window later
