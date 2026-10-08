## ADDED Requirements

### Requirement: Pause and resume controls on an object's detail view

A logged-in console user SHALL be able to pause or resume a `SignalSource`,
`Pipeline` or `Coordinator` from its detail view.

The control SHALL call the manager's `/admin/*` surface
(`operational-pause-controls`) under the console's own authentication,
never the general per-conversation chat composer.

A pause or resume action affects an object outside the conversation that
would be typing in that composer, which is the wrong blast radius for a
chat command.

On success, the detail view SHALL reflect the object's resulting `Paused`
condition without requiring a manual refresh, consistent with every other
condition the console already renders from its own watches.

On failure, the console SHALL surface the manager's refusal rather than
silently doing nothing.

#### Scenario: Pausing from the detail view
- **WHEN** a logged-in user pauses a Pipeline from its detail view
- **THEN** the console calls `/admin/pause-pipeline`, and the view shows
  `Paused=True` once the watch reports the updated object

#### Scenario: Resuming from the detail view
- **WHEN** a logged-in user resumes a paused SignalSource from its detail
  view
- **THEN** the console calls `/admin/resume-signal-source`, and the view
  shows `Paused=False` once the watch reports the updated object

#### Scenario: A refusal is surfaced, not swallowed
- **WHEN** a pause or resume call is refused by the manager
- **THEN** the console shows the refusal, and the object's displayed
  `Paused` condition is unchanged

### Requirement: An install-wide maintenance-mode switch

The console SHALL show the honored `MaintenanceMode` singleton's current
state, and SHALL let a logged-in user set or clear it.

While active, the console SHALL indicate maintenance mode across the
WHOLE install, not as a property of any single object — distinct from an
individual object's own `Paused` condition.

Where no honored singleton exists, the console SHALL surface that state
rather than offering a control that silently fails, and SHALL NOT create
one — `maintenance-mode` already states that the manager never does.

#### Scenario: Turning maintenance mode on
- **WHEN** a logged-in user sets maintenance mode from the console
- **THEN** the console calls `/admin/set-maintenance-mode`, and once the
  watch reports it, the console shows the install as under maintenance

#### Scenario: Turning maintenance mode off
- **WHEN** a logged-in user clears maintenance mode from the console
- **THEN** the console calls `/admin/clear-maintenance-mode`, and the
  install-wide indicator clears once the watch reports it

#### Scenario: A missing singleton is named, not silently refused
- **WHEN** the honored `MaintenanceMode` singleton does not exist and a
  user opens the maintenance-mode control
- **THEN** the console states that no singleton exists, rather than
  offering a toggle that would fail unexplained
