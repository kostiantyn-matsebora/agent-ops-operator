## MODIFIED Requirements

### Requirement: Selection is explicit and the confirmation names the cost
The conversation list SHALL offer per-row selection on root conversations and a select-all control scoped to the root rows on the current page.

The close action SHALL be disabled with nothing selected, and SHALL require a confirmation that states how many root conversations will be closed, how many of them are working, and that the cascade also reaches their members.

There SHALL be no control that selects conversations beyond the current page.

Every count the selection surfaces SHALL be over selected roots: the "N
selected" label, the action buttons' suffixes, the confirmation title and the
result totals. A member SHALL NOT contribute to any of them.

#### Scenario: Confirmation states the count
- **WHEN** an operator triggers the close action over four selected root conversations
- **THEN** a confirmation names the four conversations to be closed

#### Scenario: The selection label counts roots
- **WHEN** an operator selects two roots that have five members between them
- **THEN** the selection bar reads 2 selected and the close and delete buttons carry the suffix 2

#### Scenario: Nothing selected disables the action
- **WHEN** no conversation is selected
- **THEN** the close action is not available

#### Scenario: Selection cannot escape the page
- **WHEN** an operator uses the select-all control
- **THEN** only the root conversations on the current page are selected

#### Scenario: The result totals count roots
- **WHEN** a batch over three roots closes two and skips one
- **THEN** the result reads 2 closed, 1 skipped, 0 failed

### Requirement: Closed conversations are presented as a state, not an absence
The console SHALL list a `Closed` conversation, label it Closed, and distinguish it from one held by its close-topics finalizer on the way out.

A closed conversation keeps its recorded answers and costs no runtime pod and no capacity, so it is a row worth showing rather than a gap in the list.

The console SHALL use the word Closed, and never Archive or archived, in
every string it shows a person for this state: the inbox row, the thread
banner, the close confirmation and the `/close` chip.

A closed conversation SHALL NOT be typeable. That decision SHALL be read from the conversation's PHASE — which is on the CR and survives everything — and not from console-process state alone.

The console's own record that a thread was closed lives in memory for one session, so a console restart would otherwise put the composer back on every closed conversation and let a person type into one that answers each message with "this conversation is closed".

#### Scenario: A closed conversation is listed and labelled
- **WHEN** the conversation list is rendered after a close
- **THEN** the conversation appears with a `Closed` label, distinct from the `closing` label of one being deleted

#### Scenario: A closed thread says closed
- **WHEN** the operator opens a closed conversation
- **THEN** the thread banner reads that the thread was closed and offers no composer

#### Scenario: A restarted console still refuses to type into a closed conversation
- **WHEN** a console process that never observed the close renders a `Closed` conversation
- **THEN** it offers no composer
