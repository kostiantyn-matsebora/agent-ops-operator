# manager-introspection Specification

## Purpose

What the manager tells the outside world about ITSELF, as opposed to about the
objects it reconciles. The rule that shapes every requirement here: anything
readable from a Kubernetes object is read from the API server by the client under
its own RBAC, and the manager exposes only what exists nowhere else — op queue
depth, slot occupancy, cooldowns, the leader identity, and the capability
resolution that dispatch would perform.

Three surfaces carry it: `GET /status` for the specific stuck item, Prometheus
metrics for how deep and how old, and `GET /pipelines/{name}/resolved` so no
client ever reimplements composition.
## Requirements
### Requirement: The manager exposes only state that exists nowhere else
The manager SHALL expose its own runtime state and its own resolution results, and SHALL NOT expose CR snapshots. Anything readable from a Kubernetes object SHALL be read from the API server by the client under its own RBAC, never proxied through the manager.

#### Scenario: No CR proxying
- **WHEN** a client needs the spec or status of any `agentops.dev` object
- **THEN** no manager endpoint serves it, and the client reads it from the Kubernetes API under its own ServiceAccount

#### Scenario: Manager-only state is reachable
- **WHEN** a client needs state the manager holds in memory
- **THEN** an authenticated endpoint serves it, because no Kubernetes object carries it

### Requirement: Manager runtime state is served by GET /status
The manager SHALL serve `GET /status`, adapter-token authenticated, reporting at minimum: build version; the identity of the replica holding the leader lease; runtime slots in use against `MAX_ACTIVE_CONVERSATIONS`; per-adapter channel op queue depth with the oldest queued op and the oldest claimed-but-uncompleted op; and active cooldowns.

This state is in-memory by design — `OpQueue` holds pending and claimed ops in no Kubernetes object — so without this endpoint it is unobservable by any client.

#### Scenario: A stalled adapter is diagnosable
- **WHEN** an adapter claims ops and does not complete them
- **THEN** `/status` reports a growing claimed-but-uncompleted count for that adapter with the age of the oldest such op

#### Scenario: The runtime ceiling is visible
- **WHEN** runtime slots in use equal `MAX_ACTIVE_CONVERSATIONS`
- **THEN** `/status` reports the ceiling and the in-use count, so queueing is distinguishable from stalling

#### Scenario: Not a CR mirror
- **WHEN** `/status` is called
- **THEN** its payload contains no CR spec or status, only manager-held state

#### Scenario: Authenticated
- **WHEN** `/status` is called without a valid adapter token
- **THEN** the request is rejected with 401

### Requirement: Operational state is exposed as standard Prometheus metrics
The aggregates `/status` reports SHALL additionally be exposed in the Prometheus exposition format on the manager's existing metrics port, registered into the controller-runtime registry already serving it. No additional listener SHALL be introduced.

Metrics SHALL follow standard Prometheus conventions rather than a local style: an `agentops_` namespace prefix, `_total` suffixes on counters, base units with `_seconds` suffixes on durations and ages, `HELP` and `TYPE` on every series, and OpenMetrics-compatible output. Queue depths, slot occupancy and ages SHALL be gauges; hop and op outcomes SHALL be counters; run and delivery durations SHALL be histograms.

#### Scenario: Alerting without a console
- **WHEN** an adapter's op queue grows with nothing claiming, or runtime slots sit at the ceiling with waiters
- **THEN** both conditions are observable as metrics and alertable with no console running

#### Scenario: Standard exposition
- **WHEN** the metrics endpoint is scraped
- **THEN** every agent-ops series carries `HELP` and `TYPE`, uses the `agentops_` prefix, and parses as valid Prometheus/OpenMetrics text

#### Scenario: Served on the existing port
- **WHEN** the manager starts
- **THEN** agent-ops metrics appear alongside controller-runtime's on the existing metrics port, and no new port is opened

### Requirement: Metric labels are bounded by CR count
Metric labels SHALL carry only values bounded by the number of custom resources — `pipeline`, `adapter`, `source`, `channel`, `kind`, `status`, `reason`. Conversation ids, run ids, op ids and any other unbounded identifier SHALL NOT appear as labels; they identify the specific stuck item and belong to `/status`.

Metrics answer how deep, how old and how many; `/status` answers which one.

#### Scenario: No unbounded cardinality
- **WHEN** thousands of conversations have run
- **THEN** the number of metric series is unchanged, because no series is keyed by conversation, run or op

#### Scenario: The specific item is still findable
- **WHEN** a metric shows an op aged past threshold
- **THEN** `/status` names the op, its adapter and its conversation

### Requirement: Instrumentation has one emission point
Metric updates SHALL be emitted from the same call sites that emit activity events, so the ring buffer and the metrics registry are fed by one instrumentation pass. A second, independent set of instrumentation call sites SHALL NOT be introduced, because the two would drift.

#### Scenario: Stream and metrics agree
- **WHEN** a run completes
- **THEN** the activity event and the corresponding counter and histogram observation are produced together, and neither can occur without the other

### Requirement: Scrape configuration and alert rules ship with the chart, disabled by default
The chart SHALL offer an optional `VMServiceScrape` and an equivalent `ServiceMonitor`, plus example alert rules for ops queued with nothing claiming them and for runtime slots at the ceiling with waiters. All SHALL default to disabled, because neither CRD is guaranteed present in a cluster.

#### Scenario: Opt-in scraping
- **WHEN** the chart is installed at defaults
- **THEN** no `VMServiceScrape`, `ServiceMonitor` or alert rule object is created

#### Scenario: Enabling produces a working scrape
- **WHEN** the scrape object is enabled on a cluster whose CRD is present
- **THEN** it targets the manager's metrics port and the agent-ops series are collected

### Requirement: Capability resolution is served, never recomputed by clients
The manager SHALL serve `GET /pipelines/{name}/resolved`, adapter-token authenticated, returning the capabilities a Pipeline resolves to: the composed tool allowlist after composition-mode application, the effective toolsets, the effective MCP configs and servers, and the runtime that would execute it.

Clients SHALL render this verbatim and SHALL NOT reimplement composition. A second implementation of resolution would eventually disagree with the one that dispatches, and a console that can disagree with the system it observes has lost its only guarantee.

#### Scenario: Resolution matches what would run
- **WHEN** a Pipeline's resolved capabilities are requested
- **THEN** the response equals what dispatch would compose for a conversation on that Pipeline

#### Scenario: An empty allowlist is reported as empty
- **WHEN** composition yields an empty tool allowlist
- **THEN** the response reports it as empty, and no client substitutes a default

#### Scenario: Unknown pipeline
- **WHEN** the named Pipeline does not exist
- **THEN** the request is rejected with 404 rather than a partially resolved answer
