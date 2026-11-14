## MODIFIED Requirements

### Requirement: A row menu carries what acts on one conversation
Every row SHALL open a menu offering:

- mark unread, or mark read
- save, or unsave
- open in a new tab, and copy link
- open the incident, where the row is a member
- reopen, where closed
- exit runtime, and close
- delete, where closed and not saved

An action the conversation's state or the viewer's rights do not allow
SHALL be absent, never disabled without a reason.

A member conversation holds no channel binding of its own, so none of mark
unread/read, save/unsave, reopen, exit runtime, close or delete ever apply to it.
Its menu SHALL offer only open in a new tab, copy link and open the incident.

A Saved conversation SHALL carry a visible mark on its row and its thread header,
and the list SHALL offer a filter that shows only Saved conversations. Save and
unsave SHALL be absent where writes are disabled. The filter SHALL remain, being a read.

#### Scenario: A closed row offers reopen and delete
- **WHEN** the operator opens the menu on a closed conversation that is not saved
- **THEN** it offers reopen, save and delete, and not close or exit runtime

#### Scenario: A saved row offers unsave and no delete
- **WHEN** the operator opens the menu on a closed, Saved conversation
- **THEN** it offers reopen and unsave, and not delete

#### Scenario: A member row offers only navigation
- **WHEN** the operator opens the menu on a member of a coordination
- **THEN** it offers only open in a new tab, copy link and open the
  incident the member belongs to, and none of the other actions

#### Scenario: The Saved filter
- **WHEN** the operator turns on the Saved filter
- **THEN** only Saved conversations are listed, with their mark

#### Scenario: Read-only console
- **WHEN** writes are disabled
- **THEN** no save or unsave is offered, and the mark and the filter still show
