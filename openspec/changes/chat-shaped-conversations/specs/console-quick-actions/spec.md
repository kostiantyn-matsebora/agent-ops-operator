## Purpose

One-click starts and replies: chips to start a conversation with a named Pipeline or Coordinator, chips for the thread commands and for the choices a message offers, and a per-row menu of what acts on one conversation.

## ADDED Requirements

### Requirement: A conversation starts from a chip naming what answers
The view SHALL offer one chip per addressable entry of the vocabulary the
console already holds — every Ready Pipeline and Coordinator. Choosing one
SHALL open the new-conversation composer with the addressed form
`/<name> ` already typed, and the message SHALL travel the ordinary
origination path.

The chips SHALL be absent where origination is unavailable, for the same
reasons the new-conversation action is.

#### Scenario: One click addresses a pipeline
- **WHEN** the operator chooses the chip for a Pipeline
- **THEN** the composer opens with that Pipeline addressed and the cursor after it

#### Scenario: Read-only console offers no start chips
- **WHEN** writes are disabled
- **THEN** no start chip is shown

### Requirement: Thread commands and offered choices are chips
Above the composer of an open conversation the console SHALL offer the
thread-position commands as chips, with the difference between them stated
on hover. Where the latest message carries choices, each SHALL be a chip
that prefills its command.

#### Scenario: A choice becomes a reply
- **WHEN** the latest message offers two choices and the operator picks one
- **THEN** the composer holds that choice's command, ready to send

### Requirement: A row menu carries what acts on one conversation
Every row SHALL open a menu offering:

- mark unread, or mark read
- open in a new tab, and copy link
- open the incident, where the row is a member
- reopen, where closed
- exit runtime, and close
- delete, where closed

An action the conversation's state or the viewer's rights do not allow
SHALL be absent, never disabled without a reason.

#### Scenario: A closed row offers reopen and delete
- **WHEN** the operator opens the menu on a closed conversation
- **THEN** it offers reopen and delete, and not close or exit runtime

#### Scenario: A member row leads to its incident
- **WHEN** the operator opens the menu on a member of a coordination
- **THEN** it offers opening the incident the member belongs to
