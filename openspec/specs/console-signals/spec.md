# console-signals Specification

## Purpose
The console's read-only view of persisted signals: the inventory of `Signal` objects, each signal's lifecycle phase, and the per-claimant routing state a signal's detail shows. Viewing only — the console holds no write path to the kind.

## Requirements

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

### Requirement: A signal's detail shows the per-claimant routing state
A signal's detail view SHALL render the full routing record:

- every claimant with its kind, name and state (`Routing`, `Routed`, `Failed`, `Skipped`)
- for a routed claimant, the conversation it landed in, linked to that conversation's view
- for a failed or skipped claimant, the recorded reason verbatim
- for a signal in the `Dropped` phase, except a drop for reason `cooldown`, or a failed one (phases defined by `signal-persistence`), the terminal reason as loudly as the inventory shows the phase

#### Scenario: Partial routing is explained, not summarized
- **WHEN** a user opens a `PartiallyRouted` signal claimed by two pipelines
- **THEN** the detail names both claimants, shows one `Routed` with its conversation link and one non-terminal with its state

#### Scenario: A cooldown drop is not surfaced loudly
- **WHEN** a signal ends `Dropped` with reason `cooldown`
- **THEN** its detail view shows the reason as recorded, without the loud treatment a capacity or unclaimed drop gets

#### Scenario: A failure's reason is the recorded one
- **WHEN** a claimant is `Failed`
- **THEN** the detail shows the failure reason exactly as the manager recorded it, never a console-composed paraphrase

### Requirement: The signals view updates from the state feed
The signals view SHALL reflect lifecycle transitions from the manager's state read surface feed alone — no polling, no manual refresh, no Kubernetes watch.

#### Scenario: A phase change appears live
- **WHEN** an open signals view is showing a `Pending` signal and the manager routes it
- **THEN** the row's phase updates to `Routed` without a reload
