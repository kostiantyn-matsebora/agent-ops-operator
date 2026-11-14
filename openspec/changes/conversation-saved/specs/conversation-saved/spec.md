## Purpose

Lets an operator mark a conversation as kept, so that nothing reclaims it
automatically, and so that it can be found again. The mark is independent of the
conversation's phase.

## ADDED Requirements

### Requirement: A conversation can be saved and unsaved
A conversation SHALL carry an optional `status.savedAt`. Its presence means the
conversation is Saved. It SHALL be written only by the manager, and SHALL be
independent of phase: a conversation SHALL be saveable in any phase, Closed included.

Saving an already Saved conversation SHALL succeed and SHALL keep the original
`savedAt`. Unsaving a conversation that is not Saved SHALL succeed and change nothing.

#### Scenario: A closed conversation is saved
- **WHEN** an operator saves a Closed conversation
- **THEN** it records `savedAt`, stays Closed, and is shown as Saved

#### Scenario: Saving twice keeps the first time
- **WHEN** a Saved conversation is saved again
- **THEN** the request succeeds and `savedAt` is unchanged

#### Scenario: Unsaving clears the mark
- **WHEN** a Saved conversation is unsaved
- **THEN** `savedAt` is absent and the conversation is reclaimable again

### Requirement: Saving is a contract verb the manager owns
The manager SHALL serve `POST /channel/conversations/{name}/save` and
`POST /channel/conversations/{name}/unsave`. Each SHALL be accepted and processed
later, answering `202` with `invocationId` and `taskIds`. No adapter and no console
SHALL write `savedAt` to the Kubernetes API.

The thread commands `/save` and `/unsave` SHALL reach the same implementation as
the verbs.

#### Scenario: The verb accepts
- **WHEN** a channel adapter posts `save` for an existing conversation
- **THEN** the manager answers `202` and the mark is set by the task that follows

#### Scenario: A command and a verb agree
- **WHEN** `/save` is sent in a thread
- **THEN** the conversation ends up exactly as after the verb

#### Scenario: An unknown conversation
- **WHEN** `save` names a conversation that does not exist
- **THEN** the manager answers `404`

### Requirement: A saved conversation cannot be deleted
A manual `delete` of a Saved conversation SHALL be refused with `409` and SHALL
name the reason. `close`, `exit` and `reset-context` SHALL remain allowed.

#### Scenario: Delete is refused until unsaved
- **WHEN** an operator deletes a Closed, Saved conversation
- **THEN** the manager answers `409`, and after `unsave` the same request is accepted

#### Scenario: Close still works
- **WHEN** an operator closes a Saved, Idle conversation
- **THEN** it becomes Closed, remains Saved, and keeps its object and context

### Requirement: A member takes its retention from its root
A member of a coordination SHALL NOT be saved or unsaved on its own: the verbs SHALL
answer `409`. A member SHALL be treated as Saved for retention while its uncaused
root is Saved.

#### Scenario: Saving a root protects its members
- **WHEN** an uncaused root is Saved and its members are Closed past the delete window
- **THEN** no member is deleted automatically

#### Scenario: A member cannot be saved alone
- **WHEN** `save` names a member
- **THEN** the manager answers `409`
