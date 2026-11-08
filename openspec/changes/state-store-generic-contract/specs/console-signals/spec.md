## MODIFIED Requirements

### Requirement: The signals view updates from the watch
The signals view SHALL reflect lifecycle transitions from the manager's state read surface feed alone — no polling, no manual refresh, no Kubernetes watch.

#### Scenario: A phase change appears live
- **WHEN** an open signals view is showing a `Pending` signal and the manager routes it
- **THEN** the row's phase updates to `Routed` without a reload

### Requirement: A signals view lists persisted signals read-only
The console SHALL provide a signals view fed by the manager's state read surface (`GET /state/signals` and its feed), never a Kubernetes watch of the `Signal` kind:

- Each row SHALL show the signal's source, `initiationId`, phase and age.
- The view SHALL be filterable by source and by phase.
- The console SHALL expose no action on a signal — no create, no edit, no delete, no retry.

#### Scenario: A held-up signal is findable
- **WHEN** a signal sits `PartiallyRouted` while its peers routed
- **THEN** filtering the view by that phase lists it, with its source and age visible in the row

#### Scenario: No write path exists
- **WHEN** a user inspects any signal in the view
- **THEN** no control mutates the signal, and the console holds no write path to the state read surface
