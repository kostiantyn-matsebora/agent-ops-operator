# ha-signal-adapter Specification

## Purpose

The Home Assistant log signal adapter: a standalone dependency-free module
reading that instance's WebSocket API, with no Kubernetes client at all.

It reuses the cluster-events rule vocabulary exactly rather than inventing a
second one, fingerprints on Home Assistant's OWN dedup identity — logger plus
source location, never the occurrence — resumes where it stopped, and takes its
credential as environment projected per SOURCE.

Its loop breaker is the agent SURFACE: a failed agent call is logged there, and
reporting it would reach the agent that made it.

## Requirements

### Requirement: The HA signal adapter is a standalone, dependency-free module

`signals/ha/` SHALL be its own Go module with no dependencies outside this
repository, reaching Home Assistant over plain HTTP and the manager over the
`/signal/*` contract. It SHALL hold no Kubernetes client and SHALL NAME NO
ServiceAccount, because its data source is Home Assistant, not the cluster —
so it runs as the release floor, an account bound to nothing.

It SHALL normalize what it observes into signals — `fingerprint`, `labels`,
optional `title`, `payload`, `kind` — and post them to the manager's inbound
endpoint. It SHALL apply no grouping, cooldown or recurrence logic: those stay
manager-side, driven by the `SignalSource`.

#### Scenario: Adapter normalizes and posts
- **WHEN** the adapter observes a Home Assistant condition matching its configuration
- **THEN** it posts a normalized signal to the manager and performs no grouping of its own

#### Scenario: No cluster access
- **WHEN** the adapter's workload is rendered
- **THEN** it names no account of its own, runs as the release floor, and holds no Kubernetes permissions

### Requirement: Configuration reuses the cluster-events vocabulary exactly
The adapter's `spec.config` SHALL use the same two-part shape as the cluster
Events adapter: `rules` — ordered, first-match-wins, each selecting with
matchers over the signal's labels and either dropping or holding for a dwell
period after which the condition is re-checked before emitting — and `route`,
carrying inhibition.

The dwell SHALL be spelled with the Prometheus term, never the Alertmanager
batching term: they are different mechanisms and naming one after the other
would describe behaviour the borrowed system does not have.

The re-check SHALL follow the same ladder as the cluster Events adapter, with
the integration's config-entry state as the health predicate where one exists.
A record's integration identity SHALL resolve to the real Home Assistant
domain whenever the message names one, including a config-entry setup-failure
record logged under the core `homeassistant.config_entries` logger — so the
predicate can actually be looked up rather than being silently unavailable for
the one failure class it exists to confirm. Where the message names no domain,
the record SHALL fall back to its logger-derived identity, unchanged from
today. Where no config-entry predicate exists for the resolved identity, the
record SHALL be emitted only if it was **still recurring as the window
closed** — its last arrival within the closing part of the window actually
waited, the final third with a floor of thirty seconds — and dropped if it
went silent before then. A record that repeated for half a minute and then
stopped has recurred, and is the transient the dwell exists to drop. When the
log cannot be read at all, the adapter SHALL emit, failing open.

A condition describing something already COMPLETED SHALL carry a zero dwell,
because a dwell would re-check and find the recovered state, erasing the
incident. The LAST rule SHALL be a catch-all with a dwell rather than a drop, so
an unanticipated condition is verified rather than discarded.

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

### Requirement: The adapter resumes where it stopped
The adapter SHALL persist its read position through the manager's signal state
API and SHALL resume from it on restart, so a restart neither replays what it
already reported nor skips what arrived while it was down. A position the
upstream no longer accepts SHALL cause a full re-read rather than a stall.

#### Scenario: Restart resumes
- **WHEN** the adapter restarts
- **THEN** it resumes from its persisted position and does not replay already-reported conditions

#### Scenario: Stale position recovers
- **WHEN** the persisted position is no longer valid upstream
- **THEN** the adapter re-reads from the current position and continues, rather than failing repeatedly

### Requirement: The adapter never reports on agent-ops itself
The adapter SHALL NOT emit a signal about agent-ops' own machinery. Health of
the system is status, not signal: routing it back through ingest would open a
conversation with an agent whose own failure produces the next signal, and nothing downstream stops
that loop — each occurrence carries a fresh fingerprint.

#### Scenario: Self-observation produces nothing
- **WHEN** an observed condition is attributable to agent-ops' own components
- **THEN** no signal is emitted, whatever the configuration says

### Requirement: Credentials reach the adapter as environment, never through the manager
The adapter's Home Assistant credential SHALL be declared on the served
`SignalSource` and projected into the adapter pod by the reconciler, resolved by
the kubelet. The manager SHALL read no Secret in the process.

The adapter SHALL fail fast and say why when its endpoint or credential is
missing, rather than starting and silently reporting nothing.

#### Scenario: Credential is projected, not read
- **WHEN** the adapter pod starts
- **THEN** its credential arrives as environment resolved by the kubelet and the manager has performed no Secret read

#### Scenario: Missing configuration is loud
- **WHEN** the adapter starts with no endpoint configured
- **THEN** it reports the missing configuration rather than running and emitting nothing
