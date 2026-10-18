# ha-signal-adapter — delta

## MODIFIED Requirements

### Requirement: The HA signal adapter is a standalone, dependency-free module

`signals/ha/` SHALL be its own Go module with no dependencies outside this
repository, reaching Home Assistant over plain HTTP and the manager over the
`/signal/*` contract.

It SHALL hold no Kubernetes client and SHALL NAME NO ServiceAccount, because
its data source is Home Assistant, not the cluster — so it runs as the
release floor, an account bound to nothing.

It SHALL normalize what it observes into signals — `initiationId`, `labels`,
optional `title`, `payload`, `kind` — and post them to the manager's inbound
endpoint.

The adapter SHALL mint each signal's `initiationId` in its existing pending
and dwell logic — the point where it already decides what constitutes one
problem.

The id keys on the condition's own identity, never the occurrence: logger
plus source location for a log record, the condition's identity for a
surface. Cooldown and routing stay manager-side, keyed on that id.

#### Scenario: Adapter normalizes and posts
- **WHEN** the adapter observes a Home Assistant condition matching its configuration
- **THEN** it posts a normalized signal carrying the `initiationId` its own dwell logic minted, and no manager-side policy regroups it

#### Scenario: A recurring condition repeats its id
- **WHEN** a condition already reported recurs and is emitted again, outside the source's cooldown and while its conversation is open
- **THEN** the signal carries the same `initiationId` and lands on that conversation as a recurrence

#### Scenario: No cluster access
- **WHEN** the adapter's workload is rendered
- **THEN** it names no account of its own, runs as the release floor, and holds no Kubernetes permissions

### Requirement: The adapter observes Home Assistant's health surfaces beside its log
The adapter SHALL observe four further surfaces of the instance it serves and
normalize each into the same signal shape a log record produces, on the same
`SignalSource`, so one route and one rules policy cover everything the
instance reports about its own health:

| Surface | One signal per | Correlation id (`initiationId`) | Message |
|---|---|---|---|
| a config entry in a failed state (`setup_retry`, `setup_error`, `migration_error` by default) | entry | domain and entry id | the entry's own reason |
| a repair issue | issue | domain and issue id | the issue's translation key and placeholders |
| a `binary_sensor` of device class `problem` in state `on`, or `connectivity` in state `off` (by default) | entity | entity id | the entity's name, state and device class |
| pending updates — `update.*` entities in state `on` | the source (ONE digest) | the source | every pending entity with its installed and latest version |

Every signal, log records included, SHALL carry a `surface` label — `log`,
`config-entry`, `repair`, `sensor` or `update`. The `integration` label SHALL
name the integration the condition belongs to, so a rule can select a surface
and a reader can tell integrations apart.

Repair severity SHALL map to the `level` label. The `levels` configuration
SHALL apply to log records only — a surface is silenced by disabling it or by
a rule.

Each surface SHALL be enable-able independently in the source configuration,
the first three enabled by default and the update digest disabled by default.
A disabled surface SHALL cause no read on its own account.

The one read two consumers share is the config entry listing, which the log
lane's dwell re-check already uses as its health predicate. With the
config-entry surface disabled it SHALL be read only while a dwell is pending,
and never otherwise.

The set of config entry states, repair severities and sensor device classes
that count SHALL each be configurable, with the defaults above.

A standing condition SHALL be reported when it appears, and again only after
it went away and came back. The update digest SHALL be re-posted when a new
pending update joins the set, carrying the whole set.

Where the adapter restarts, re-reporting what still stands is permitted: the
re-report repeats the same `initiationId`, so the manager's cooldown absorbs
it or the open conversation receives it as a recurrence.

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
- **THEN** the adapter issues no command on that surface's account and emits nothing from it

#### Scenario: Recovery and recurrence
- **WHEN** a reported condition goes away and later comes back
- **THEN** it is reported again, while it merely stands it is not
