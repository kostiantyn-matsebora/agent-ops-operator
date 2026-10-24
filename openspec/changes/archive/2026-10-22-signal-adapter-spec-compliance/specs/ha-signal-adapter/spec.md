# ha-signal-adapter delta

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

The id's scope follows the path that emitted the signal, never the
occurrence:

| Path | The id keys on |
|---|---|
| a dwelled condition | the source plus the INTEGRATION the condition belongs to — the dwell queue's own "same problem" key, stable across every dwell cycle the trouble recurs through |
| a zero-dwell rule | the source plus the record's own identity — logger and source location for a log record, the surface's condition id for a surface condition |

Cooldown and routing stay manager-side, keyed on that id.

#### Scenario: Adapter normalizes and posts
- **WHEN** the adapter observes a Home Assistant condition matching its configuration
- **THEN** it posts a normalized signal carrying the `initiationId` its own dwell logic minted, and no manager-side policy regroups it

#### Scenario: A recurring condition repeats its id
- **WHEN** a condition already reported recurs and is emitted again, outside the source's cooldown and while its conversation is open
- **THEN** the signal carries the same `initiationId` and lands on that conversation as a recurrence

#### Scenario: A dwelled integration keeps one id across cycles
- **WHEN** one integration's recurring trouble passes two dwell windows, each cycle sampling a different log record
- **THEN** both emitted signals carry the same source-and-integration id, so the history stays in one conversation

#### Scenario: No cluster access
- **WHEN** the adapter's workload is rendered
- **THEN** it names no account of its own, runs as the release floor, and holds no Kubernetes permissions

### Requirement: Configuration reuses the cluster-events vocabulary exactly
The adapter's `spec.config` SHALL use the same two-part shape as the cluster
Events adapter:

- `rules` — ordered, first-match-wins, each selecting with matchers over the
  signal's labels, and either dropping or holding for a dwell period after
  which the condition is re-checked before emitting.
- `route` — carrying inhibition.

The dwell SHALL be spelled with the Prometheus term, never the Alertmanager
batching term: they are different mechanisms and naming one after the other
would describe behaviour the borrowed system does not have.

The re-check SHALL follow the same ladder as the cluster Events adapter, with
the integration's config-entry state as the health predicate where one exists.

A record's integration identity SHALL resolve to the real Home Assistant
domain whenever the message names one.

A config-entry setup-failure record logged under the core
`homeassistant.config_entries` logger resolves the same way, so the predicate
can actually be looked up for the one failure class it exists to confirm.

Where the message names no domain, the record SHALL fall back to its
logger-derived identity, unchanged from today.

Where no config-entry predicate exists for the resolved identity, the record
SHALL be emitted only if it was **still recurring as the window closed**, and
dropped if it went silent before then.

"Still recurring" means its last arrival fell within the closing part of the
window actually waited — the final third, with a floor of thirty seconds.

A record that repeated for half a minute and then stopped has recurred, and
is the transient the dwell exists to drop.

The quiet-drop verdict SHALL apply only where the log could actually be
observed through the window. When the log cannot be read at the close, the
adapter SHALL emit, failing open.

An unreadable Home Assistant is itself an incident, never quiet churn. No
other verdict SHALL swallow that emit.

A condition describing something already COMPLETED SHALL carry a zero dwell,
because a dwell would re-check and find the recovered state, erasing the
incident.

The LAST rule SHALL be a catch-all with a dwell rather than a drop, so an
unanticipated condition is verified rather than discarded.

#### Scenario: First match wins
- **WHEN** an observed condition matches two rules
- **THEN** the earlier rule decides and the later one is not consulted

#### Scenario: Completed conditions do not dwell
- **WHEN** the shipped defaults are inspected
- **THEN** every rule describing a completed condition carries a zero dwell

#### Scenario: The unanticipated is verified, not dropped
- **WHEN** a condition matches no earlier rule
- **THEN** the catch-all holds it for its dwell and re-checks, rather than discarding it

#### Scenario: A network blip that logged for thirty seconds is churn
- **WHEN** an integration with no config-entry predicate logs the same error repeatedly for thirty seconds and then stops, under a rule with a three-minute dwell
- **THEN** no signal is emitted

#### Scenario: An integration still logging at the close is reported
- **WHEN** an integration with no config-entry predicate keeps logging the same error through the whole dwell
- **THEN** one signal is emitted at the deadline, naming when the last record arrived

#### Scenario: An unreadable log at the close fails open
- **WHEN** a dwelled record's window closes while the system log cannot be read
- **THEN** one signal is emitted, rather than the record being dropped for want of observed recurrence

#### Scenario: A config-entry setup failure resolves to its domain
- **WHEN** Home Assistant logs a config-entry setup failure under the
  `homeassistant.config_entries` logger naming an integration domain in its
  message (for example "Error setting up entry ... for tuya")
- **THEN** the record's integration identity is that domain, not the logger
  name, and the domain's config-entry state is used as the health predicate

#### Scenario: A still-broken config entry is confirmed, not dropped as quiet
- **WHEN** a config-entry setup failure is logged once, its domain's config
  entry remains in a failed state for the whole dwell window, and no second
  matching log line arrives
- **THEN** the health predicate confirms it as still broken and one signal is
  emitted at the deadline, rather than the record being dropped for want of
  recurrence

#### Scenario: An unparseable config-entry message keeps today's behavior
- **WHEN** Home Assistant logs a `homeassistant.config_entries` record whose
  message names no recognizable domain
- **THEN** the record's integration identity falls back to the logger name,
  and it is verified by recurrence exactly as before this change

### Requirement: The adapter observes Home Assistant's health surfaces beside its log
The adapter SHALL observe four further surfaces of the instance it serves and
normalize each into the same signal shape a log record produces, on the same
`SignalSource`, so one route and one rules policy cover everything the
instance reports about its own health:

| Surface | One signal per | `initiationId` under the shipped defaults | Message |
|---|---|---|---|
| a config entry in a failed state (`setup_retry`, `setup_error`, `migration_error` by default) | entry | the entry's domain, source-scoped — the shipped rules dwell this surface, so the id takes the dwelled path | the entry's own reason |
| a repair issue | issue | domain and issue id — the shipped rules give repairs a zero dwell, so the condition's own identity applies | the issue's translation key and placeholders |
| a `binary_sensor` of device class `problem` in state `on`, or `connectivity` in state `off` (by default) | entity | the owning platform, source-scoped — dwelled like config entries | the entity's name, state and device class |
| pending updates — `update.*` entities in state `on` | the source (ONE digest) | the source | every pending entity with its installed and latest version |

The id column states the shipped defaults, not a property of the surface. A
rule set moving a surface between the dwelled and zero-dwell paths moves its
id scope with it, per the module requirement's two paths.

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

#### Scenario: A dwelled surface correlates on its integration
- **WHEN** one domain's config entry fails, recovers, and fails again days later under the shipped dwell defaults
- **THEN** both emitted signals carry the same source-and-domain id, landing as one conversation's recurrence rather than two conversations

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
- **THEN** it is reported again — while it merely stands it is not

## ADDED Requirements

### Requirement: The dwell queue declares a post-emit refractory window

After emitting a condition's signal, the adapter SHALL suppress its own
re-posts of that condition — the same `initiationId` — for the emitting
rule's own dwell, floored at one minute.

This is the signal adapter contract's one named allowance, declared here as
that allowance requires. The window opens only on emit, never collapses
distinct ids, and substitutes for nothing: the manager's cooldown stays the
dedup authority, and the window only spends fewer round trips.

#### Scenario: Chatter inside the window is not re-posted
- **WHEN** an emitted condition keeps recurring within its refractory window
- **THEN** no further post is made for that id, and nothing the manager keeps
  differs from what its cooldown would have produced

#### Scenario: Recurrence past the window lands as a recurrence
- **WHEN** the condition still recurs after the window closes
- **THEN** the adapter posts again under the same `initiationId` and the
  manager appends it to the open conversation
