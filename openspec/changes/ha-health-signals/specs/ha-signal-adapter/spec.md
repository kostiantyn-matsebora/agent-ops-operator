## ADDED Requirements

### Requirement: The adapter observes Home Assistant's health surfaces beside its log
The adapter SHALL observe four further surfaces of the instance it serves and
normalize each into the same signal shape a log record produces, on the same
`SignalSource`, so one route and one rules policy cover everything the
instance reports about its own health:

| Surface | One signal per | Fingerprint | Message |
|---|---|---|---|
| a config entry in a failed state (`setup_retry`, `setup_error`, `migration_error` by default) | entry | domain and entry id | the entry's own reason |
| a repair issue | issue | domain and issue id | the issue's translation key and placeholders |
| a `binary_sensor` of device class `problem` in state `on`, or `connectivity` in state `off` (by default) | entity | entity id | the entity's name, state and device class |
| pending updates — `update.*` entities in state `on` | the source (ONE digest) | the source | every pending entity with its installed and latest version |

Every signal, log records included, SHALL carry a `surface` label — `log`,
`config-entry`, `repair`, `sensor` or `update` — and the `integration` label
SHALL name the integration the condition belongs to, so a rule can select a
surface and grouping stays by integration. Repair severity SHALL map to the
`level` label. The `levels` configuration SHALL apply to log records only; a
surface is silenced by disabling it or by a rule.

Each surface SHALL be enable-able independently in the source configuration,
the first three enabled by default and the update digest disabled by default,
and a disabled surface SHALL cause no read of that surface at all. The set of
config entry states, repair severities and sensor device classes that count
SHALL each be configurable, with the defaults above.

A standing condition SHALL be reported when it appears, and again only after
it went away and came back. The update digest SHALL be re-posted when a new
pending update joins the set, carrying the whole set. Where the adapter
restarts, re-reporting what still stands is permitted: the manager's
fingerprint cooldown and signature window absorb it.

#### Scenario: An integration that stopped working without logging is reported
- **WHEN** a config entry enters `setup_retry` with a reason and no log record is written
- **THEN** one signal is emitted for that entry, `surface=config-entry`, `integration` naming its domain, with the reason as its message

#### Scenario: A repair is reported once, with what it names
- **WHEN** the issue registry holds an issue of a counting severity
- **THEN** one signal is emitted for it, `surface=repair`, `level` from its severity, its placeholders in the payload, and it is not emitted again while it stands

#### Scenario: A device fault reported as state is reported
- **WHEN** a `binary_sensor` of device class `problem` turns on
- **THEN** one signal is emitted, `surface=sensor`, `integration` naming the platform that owns the entity

#### Scenario: Updates are one digest, and off unless asked for
- **WHEN** several `update.*` entities are in state `on` and the update surface is not enabled
- **THEN** nothing is emitted
- **WHEN** the update surface is enabled
- **THEN** exactly one signal is emitted for the source, listing every pending update, and it is emitted again only when a new one joins the set

#### Scenario: A disabled surface is not read
- **WHEN** a surface is disabled in the source configuration
- **THEN** the adapter issues no command for that surface and emits nothing from it

#### Scenario: Recovery and recurrence
- **WHEN** a reported condition goes away and later comes back
- **THEN** it is reported again; while it merely stands it is not

### Requirement: Surface conditions pass the same policy as log records
A condition from any surface SHALL pass the same path a log record does —
self-exclusion, the ordered first-match rules, inhibition and the dwell — so
a rule may drop or hold it by its `surface`, `integration` or `level` label.
At the close of a dwell the re-check SHALL consult the surface's own current
truth: a config entry still in a failed state, an issue still filed, a sensor
still faulting — and a condition that cleared before the close SHALL be
dropped as the churn it was. Self-exclusion's marker and own-user mechanisms
SHALL apply to every surface; the agent-surface mechanism concerns loggers and
applies to log records alone.

#### Scenario: A restart's churn is dropped
- **WHEN** a config entry passes through `setup_retry` during a Home Assistant restart and is loaded before the rule's dwell closes
- **THEN** no signal is emitted

#### Scenario: A surface can be silenced by rule
- **WHEN** a rule matches `surface="sensor"` with `action: drop`
- **THEN** no sensor condition is emitted, and every other surface is unaffected

#### Scenario: A repair about agent-ops itself produces nothing
- **WHEN** a repair issue's text names agent-ops or the adapter's own user
- **THEN** no signal is emitted, whatever the configuration says
