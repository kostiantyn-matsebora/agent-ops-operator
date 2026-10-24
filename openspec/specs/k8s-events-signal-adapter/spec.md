# k8s-events-signal-adapter

## Purpose

The Kubernetes events signal adapter in `signals/k8s-events/`: a dependency-free module that serves SignalSources naming its `SignalAdapter` CR through the signal adapter contract, watching core `v1` Events with its own ServiceAccount token and normalizing them into `kind: alert` signals with a deterministic workload-keyed `initiationId`, so cooldown, routing, and recurrence stay manager-side.

## Requirements

### Requirement: Kubernetes events run as a signal adapter
A signal adapter in `signals/k8s-events/` (own dependency-free Go module and image, precedents `signals/cron/` and `channels/telegram/`) SHALL serve SignalSources whose `spec.adapter` names its `SignalAdapter` CR (default name `k8s-events`) through the signal adapter contract, watching core `v1` Events via the in-cluster Kubernetes API using its own ServiceAccount token (no client-go). Per-source `config` SHALL support `severities` (Event `type` values, default `["Warning"]`), `namespaces` (empty or absent = all namespaces the granted RBAC allows), the ordered `rules` and `route` stanzas defined by the `k8s-event-suppression` capability, and the legacy `includeReasons`/`excludeReasons` exact-match filters, which SHALL remain accepted and translate to equivalent rules. Invalid config (unknown severity value, malformed matcher, unparsable duration, or malformed shape) SHALL be reported as a False Ready condition on that source via the contract status API while other sources keep being served. No signal-type constant SHALL be added to `api/v1alpha1`: the adapter CR's NAME is the routing key, and there are no built-in signal types.

To resolve workload identity and to evaluate liveness, the adapter SHALL additionally maintain a read-only list/watch cache of `pods` and `replicasets`, retaining only the fields it needs (owner references, phase, `Ready` condition, container waiting reasons, and selected labels) rather than whole objects. The permissions for that cache SHALL be granted externally against the adapter's ServiceAccount, as with its Events access — the operator grants adapters nothing. When the grant is absent, the adapter SHALL report `Ready=False` naming the missing permission rather than degrading silently.

#### Scenario: Warning events flow once the adapter serves a source
- **WHEN** a SignalSource whose `adapter` names the k8s-events adapter is served with empty `config` and a pod in the cluster emits a `Warning` event (e.g. `BackOff`)
- **THEN** a normalized signal for that event is posted to `/signal/inbound` and a conversation is created through the manager's ingest pipeline

#### Scenario: Severity filter honors configuration
- **WHEN** a source's `config.severities` is `["Normal", "Warning"]`
- **THEN** events of both types produce signals, while a source with default config produces signals only for `Warning` events

#### Scenario: Invalid severity surfaces on the source
- **WHEN** a source's `config.severities` contains a value that is not a valid Event type
- **THEN** the adapter sets a False Ready condition naming the problem and other sources keep producing signals

#### Scenario: Missing pod permissions are reported, not hidden
- **WHEN** the adapter's ServiceAccount lacks `list`/`watch` on `pods`
- **THEN** every served source reports `Ready=False` naming the missing grant

### Requirement: Events normalize with a workload-keyed correlation id
The adapter SHALL emit `kind: alert` signals with a deterministic
`initiationId` of `<source>@<namespace>/<workload>` — stable across restarts,
event-object recreation and rollouts — so every failure of one workload
correlates to one conversation.

Minting the id IS the grouping decision, and it is made here because the
adapter already resolves workload identity and already batches per workload.
Cooldown, routing and recurrence remain manager-side, keyed on that id.

Signals SHALL carry labels covering at least `alertgroup: k8s-events`,
`alertname: <reason>`, `namespace`, `kind`, `name`, `severity`, `workload`,
and `node`, which rules matchers, inhibition and display select on.

The `workload` label — and with it the id — SHALL be resolved through owner
references (Pod → ReplicaSet → Deployment, and the equivalent chain for other
controllers), never by parsing the object's name.

Name-based inference breaks on StatefulSets, DaemonSets, and bare pods.

When no controller owns the object, `workload` SHALL name the object itself.

Resolution lag SHALL NOT re-mint per-object grouping. When a tracked kind's
owning workload cannot yet be resolved because the adapter's own cache has
not caught up, the adapter SHALL hold the event for a short bounded wait
of one 30-second cache refresh tick, which is not configurable.
The adapter SHALL NOT emit under the object's own name during that wait.

The object-keyed id applies only to an object confirmed unowned, or to one
still unresolved when that wait expires — so a momentary cache lag delays the
signal, and never loses it.

The adapter SHALL apply suppression per the `k8s-event-suppression`
capability. Suppression is filtering, which has always been the adapter's
role, and is distinct from correlation.

#### Scenario: Crash-loop repeats collapse into one conversation
- **WHEN** the same pod emits repeated `BackOff` warning events
- **THEN** all occurrences carry the same `initiationId` and land in (or are suppressed by cooldown into) the existing conversation rather than spawning new ones

#### Scenario: One workload, one conversation, across rollouts
- **WHEN** pods of one Deployment fail across several rollouts
- **THEN** every failure carries the same workload-keyed id and shares one conversation, which persists across rollouts because the workload identity does not change with pod names

#### Scenario: Workload comes from owner references
- **WHEN** an event's involved object is a pod named `app-0` owned by a StatefulSet, or `app-xk2p9` owned by a DaemonSet
- **THEN** the id keys on the owning controller in both cases, with no dependence on the shape of the pod name

#### Scenario: A cache catching up does not split a workload's conversation
- **WHEN** a pod's event arrives before the adapter's cache holds its owning ReplicaSet, and the owner appears within the bounded wait
- **THEN** the emitted signal carries the workload-keyed id, never a per-pod one

#### Scenario: The wait expires without an owner
- **WHEN** a pod's event arrives, and its owner is still unresolved after the 30-second wait
- **THEN** the adapter emits the signal with the object-keyed id, so the signal is delayed and never lost

#### Scenario: An unowned pod is its own workload
- **WHEN** an event's involved object is a bare pod with no owner references
- **THEN** the id keys on that pod

#### Scenario: Distinct workloads never merge
- **WHEN** two unrelated workloads fail in one namespace
- **THEN** their signals carry distinct `initiationId` values and open distinct conversations

### Requirement: The object cache also holds nodes
The read-only list/watch cache SHALL additionally hold nodes, retaining only
name, `spec.unschedulable` and taints. Node access SHALL be granted externally
like the rest; absent, the adapter SHALL run with drain awareness off and
report that on each source without `Ready=False`, since nodes are
cluster-scoped and a namespaced install has deliberately no view of them.

#### Scenario: Nodes are cached from fields
- **WHEN** a node is cordoned
- **THEN** the cache reflects `unschedulable` within the watch's latency without holding the node's status

#### Scenario: Missing node access is not a failure
- **WHEN** the ServiceAccount lacks `list`/`watch` on `nodes`
- **THEN** sources stay `Ready=True` and their condition names drain awareness as unavailable

### Requirement: Watching is restart-safe
The adapter SHALL persist a per-source cursor (the maximum event `lastTimestamp` observed) through the contract state API and skip already-seen events on startup's initial list; on watch expiry (`410 Gone`) or stream error it SHALL relist and resume. Delivery is at-least-once: duplicates after an ill-timed restart are acceptable because deterministic fingerprints collapse under the manager's cooldown.

#### Scenario: Restart does not replay old events
- **WHEN** the adapter restarts and the initial Events list returns events at or before the persisted cursor
- **THEN** no signals are emitted for those events

#### Scenario: Watch expiry recovers without losing new events
- **WHEN** the API server ends the watch with `410 Gone`
- **THEN** the adapter relists, resumes watching, and events emitted after the expiry still produce signals

### Requirement: The adapter never signals on agent-ops' own objects
The adapter SHALL implement the `signal-self-exclusion` invariant: events whose involved object belongs to agent-ops are dropped before any rule is evaluated, by the owner/label rule, the name-prefix rule, and release-namespace exclusion independently. The first two SHALL NOT be configurable.

#### Scenario: A failing runtime pod does not create a conversation
- **WHEN** a runtime pod cannot be scheduled and emits `FailedScheduling` warnings under successive pod names
- **THEN** no signal is emitted for any of them, and the create-fail-create cycle does not start
