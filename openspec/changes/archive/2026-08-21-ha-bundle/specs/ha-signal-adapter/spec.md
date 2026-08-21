# ha-signal-adapter

## ADDED Requirements

### Requirement: The HA signal adapter is a standalone, dependency-free module
`signal-ha/` SHALL be its own Go module with no dependencies outside this
repository, reaching Home Assistant over plain HTTP and the manager over the
`/signal/*` contract. It SHALL hold no Kubernetes client and SHALL declare
`kubernetesAccess: false`, because its data source is Home Assistant, not the
cluster.

It SHALL normalize what it observes into signals — `fingerprint`, `labels`,
optional `title`, `payload`, `kind` — and post them to the manager's inbound
endpoint. It SHALL apply no grouping, cooldown or recurrence logic: those stay
manager-side, driven by the `SignalSource`.

#### Scenario: Adapter normalizes and posts
- **WHEN** the adapter observes a Home Assistant condition matching its configuration
- **THEN** it posts a normalized signal to the manager and performs no grouping of its own

#### Scenario: No cluster access
- **WHEN** the adapter's workload is rendered
- **THEN** it mounts no ServiceAccount token and holds no Kubernetes permissions

### Requirement: Configuration reuses the cluster-events vocabulary exactly
The adapter's `spec.config` SHALL use the same two-part shape as the cluster
Events adapter: `rules` — ordered, first-match-wins, each selecting with
matchers over the signal's labels and either dropping or holding for a dwell
period after which the condition is re-checked before emitting — and `route`,
carrying inhibition.

The dwell SHALL be spelled with the Prometheus term, never the Alertmanager
batching term: they are different mechanisms and naming one after the other
would describe behaviour the borrowed system does not have.

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
the system is status, not signal: routing it back through ingest would wake an
agent whose own failure produces the next signal, and nothing downstream stops
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
