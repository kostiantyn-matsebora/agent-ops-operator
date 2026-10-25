# console-signals

## Purpose

The console's read-only view of persisted signals: the inventory of `Signal` objects, each signal's lifecycle phase, and the per-claimant routing state a signal's detail shows. Viewing only — the console holds no write path to the kind.

## ADDED Requirements

### Requirement: A signals view lists persisted signals read-only
The console SHALL provide a signals view fed by its read-only watch of the `Signal` kind:

- Each row SHALL show the signal's source, `initiationId`, phase, occurrence count and age.
- The view SHALL be filterable by source and by phase.
- The console SHALL expose no action on a signal — no create, no edit, no delete, no retry.

#### Scenario: A held-up signal is findable
- **WHEN** a signal sits `PartiallyRouted` while its peers routed
- **THEN** filtering the view by that phase lists it, with its source and age visible in the row

#### Scenario: No write path exists
- **WHEN** a user inspects any signal in the view
- **THEN** no control mutates the signal, and the console's ServiceAccount holds no write verb on the kind

### Requirement: A signal's detail shows the per-claimant routing state
A signal's detail view SHALL render the full routing record:

- every claimant with its kind, name and state (`Routing`, `Routed`, `Failed`, `Skipped`)
- for a routed claimant, the conversation it landed in, linked to that conversation's view
- for a failed or skipped claimant, the recorded reason verbatim
- for a dropped or failed signal, the terminal reason as loudly as the inventory shows the phase

#### Scenario: Partial routing is explained, not summarized
- **WHEN** a user opens a `PartiallyRouted` signal claimed by two pipelines
- **THEN** the detail names both claimants, shows one `Routed` with its conversation link and one non-terminal with its state

#### Scenario: A failure's reason is the recorded one
- **WHEN** a claimant is `Failed`
- **THEN** the detail shows the failure reason exactly as the manager recorded it, never a console-composed paraphrase

### Requirement: The signals view updates from the watch
The signals view SHALL reflect lifecycle transitions from watch events alone — no polling, no manual refresh.

#### Scenario: A phase change appears live
- **WHEN** an open signals view is showing a `Pending` signal and the manager routes it
- **THEN** the row's phase updates to `Routed` without a reload
