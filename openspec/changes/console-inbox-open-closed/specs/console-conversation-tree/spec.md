## MODIFIED Requirements

### Requirement: Closing a root is shown as closing its members
The manager cascades a close through every live descendant. The confirmation for closing a selection SHALL count the selected ROOT conversations in its title and SHALL NOT add their descendants to that count.

Where the selected roots have descendants, the confirmation SHALL state in a separate line how many descendants close with them, at every depth, and how many of those are working.

A member SHALL NOT be selectable in this view and SHALL NOT be closable on its
own. The row menu SHALL NOT offer close for a member row. The select-all
control SHALL select root conversations only, and SHALL skip a root already closing or being deleted.

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

A delete request that names a member directly SHALL report that name skipped, with a reason that it is a member and naming its parent, and SHALL NOT act on it.

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
