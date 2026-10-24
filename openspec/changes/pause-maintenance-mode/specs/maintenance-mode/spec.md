# maintenance-mode

## Purpose

`MaintenanceMode` is the install-wide switch that holds the manager's
ingest and admission paths during a deliberate pause — a maintenance window
or an incident — without evicting any conversation already running.

## ADDED Requirements

### Requirement: MaintenanceMode is a namespaced singleton named `default`

The `MaintenanceMode` CRD SHALL carry one field, `spec.paused` (bool,
default `false`, mutable).

The manager SHALL honor exactly one `MaintenanceMode` object: the one named
`default` in the manager's own release namespace. No admission webhook
SHALL enforce this — it is read-path discipline, applied by the reconciler
and by every caller that consults `MaintenanceMode` state.

Any other `MaintenanceMode` object in that namespace SHALL be accepted by
the API server (the kind carries no uniqueness constraint beyond the name)
but SHALL be ignored for enforcement, and SHALL report a status condition
naming the one that is honored.

The absence of any `MaintenanceMode` object SHALL read as not-paused —
nothing elsewhere requires one to exist.

#### Scenario: The singleton is honored
- **WHEN** a `MaintenanceMode` named `default` exists in the manager's namespace
- **THEN** its `spec.paused` is the value the manager enforces install-wide

#### Scenario: A second object is accepted and ignored
- **WHEN** a `MaintenanceMode` named anything other than `default` is created
- **THEN** the API server accepts it, the manager enforces nothing from its
  `spec.paused`, and its status names `default` as the one actually honored

#### Scenario: Absence means not paused
- **WHEN** no `MaintenanceMode` object exists at all
- **THEN** the manager behaves exactly as if the honored singleton had
  `spec.paused: false`

### Requirement: The honored singleton reports whether it is in effect

`MaintenanceMode.status.conditions` SHALL carry at least two conditions:

- `Honored` — `True` when this object is the one named `default` in the
  manager's own namespace, `False` otherwise, naming the honored object.
- `Paused` — the SAME condition type `SignalSource`, `Pipeline` and
  `Coordinator` carry (`operational-pause-controls`), reporting whether
  maintenance mode is CURRENTLY IN EFFECT: `True` only on the honored
  singleton with `spec.paused: true`. On an object that is not honored,
  `Paused` SHALL be `False` regardless of that object's own `spec.paused`,
  naming that it is not the honored one — its `spec.paused` is never
  enforced and must not be read as if it were.

#### Scenario: The honored singleton reflects its own field
- **WHEN** the `default` object's `spec.paused` is set to `true`
- **THEN** its `Paused` condition becomes `True`

#### Scenario: An unhonored object never reports active
- **WHEN** a `MaintenanceMode` not named `default` has `spec.paused: true`
- **THEN** its `Paused` condition is `False`, naming that it is not honored

### Requirement: The chart renders exactly one singleton, unpaused, by default

A release SHALL render exactly one `MaintenanceMode` object, named
`default`, with `spec.paused: false` — so a fresh install ships with
maintenance mode off and nothing an operator must declare to get that
behavior.

#### Scenario: A fresh install ships unpaused
- **WHEN** a release is installed with no maintenance-mode value overridden
- **THEN** the `default` `MaintenanceMode` object exists with `spec.paused: false`

### Requirement: Active maintenance mode drops every inbound signal before any per-source check

While the honored singleton has `spec.paused: true`, the manager SHALL drop
EVERY inbound signal at `/signal/inbound` — regardless of which
`SignalSource` it names — before any per-source check (the source's own
`Wired` condition, its own `paused` field, cooldown or grouping).

The response SHALL report the batch as not queued, carrying a reason that
names maintenance mode, distinct from an unwired-source or a paused-source
reason.

Once maintenance mode is cleared, signals resume routing exactly per each
source's own `Wired` and `paused` state — clearing maintenance mode does
NOT resume a source that was independently paused.

#### Scenario: A signal is dropped while maintenance mode is active
- **WHEN** maintenance mode is active and a signal arrives for a SignalSource
  that is otherwise `Wired=True` and not itself paused
- **THEN** no conversation or input is created, and the response names
  maintenance mode as the reason — not the source's own state

#### Scenario: Clearing maintenance mode resumes ordinary routing
- **WHEN** maintenance mode is cleared
- **THEN** a subsequent signal for that same source routes exactly as it
  would have if maintenance mode had never been active

#### Scenario: A paused source stays paused after maintenance mode clears
- **WHEN** maintenance mode is cleared and the signal's own SignalSource has
  `spec.paused: true`
- **THEN** the signal is still dropped, now for the source's own reason
  (`signal-source-model`), never mistaken for maintenance mode

### Requirement: Active maintenance mode holds unadmitted conversations Pending, without provisioning

While the honored singleton has `spec.paused: true`, a conversation that
needs a worker and has no runtime pod yet SHALL be held in phase `Pending`,
with a condition naming maintenance mode as the reason — mirroring the
storage breaker's existing conservative admission hold
(`conversation_controller.go`'s `enterPendingFor`).

While so held, the manager SHALL create no runtime pod, no MCP ConfigMap,
and no chat topic (`ensure-topic`) for that conversation.

A conversation that is ALREADY pod-backed when maintenance mode becomes
active SHALL be left entirely alone. It keeps running, is never evicted,
and its pod is never torn down on account of maintenance mode.

This holds whether the conversation was admitted before maintenance mode
became active, or let through as ordinary admission racing the toggle.

#### Scenario: A new conversation is held
- **WHEN** maintenance mode is active and a signal would otherwise open a
  new conversation needing a worker
- **THEN** the conversation exists in phase `Pending`, naming maintenance
  mode as the reason, with no runtime pod, no MCP ConfigMap and no chat
  topic created for it

#### Scenario: A running conversation is left to finish
- **WHEN** maintenance mode becomes active while a conversation already has
  a runtime pod
- **THEN** that conversation's pod is untouched and the conversation
  continues to completion

#### Scenario: Pending conversations are admitted once maintenance mode clears
- **WHEN** maintenance mode is cleared
- **THEN** conversations held `Pending` for that reason become eligible for
  ordinary admission again, in the same FIFO order admission already uses
