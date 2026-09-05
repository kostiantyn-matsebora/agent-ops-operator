# k8s-events-signal-adapter

## Purpose

The Kubernetes events signal adapter in `signals/k8s-events/`: a dependency-free module that serves SignalSources naming its `SignalAdapter` CR through the signal adapter contract, watching core `v1` Events with its own ServiceAccount token and normalizing them into `kind: alert` signals with deterministic fingerprints, so grouping, cooldown, and recurrence stay manager-side.
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

### Requirement: Events normalize with stable fingerprints for manager-side grouping
The adapter SHALL emit `kind: alert` signals with a deterministic fingerprint `<source>@<namespace>/<involvedObject.kind>/<involvedObject.name>/<reason>` (stable across restarts and event-object recreation) and labels covering at least `alertgroup: k8s-events`, `alertname: <reason>`, `namespace`, `kind`, `name`, `severity`, `workload`, and `node`, so `SignalSource.spec.grouping` can group by workload, object, reason, or namespace.

The `workload` label SHALL be resolved through owner references (Pod → ReplicaSet → Deployment, and the equivalent chain for other controllers), never by parsing the object's name — name-based inference breaks on StatefulSets, DaemonSets, and bare pods. When no controller owns the object, `workload` SHALL name the object itself.

The adapter SHALL NOT group signals or apply cooldown beyond its restart cursor — grouping, cooldown, window reuse, and recurrence remain manager-side. It SHALL apply suppression per the `k8s-event-suppression` capability: suppression is filtering, which has always been the adapter's role, and is distinct from grouping.

#### Scenario: Crash-loop repeats collapse into one conversation
- **WHEN** the same pod emits repeated `BackOff` warning events within the source's cooldown and grouping window
- **THEN** all occurrences carry the same fingerprint and land in (or are suppressed into) the existing conversation rather than spawning new ones

#### Scenario: Labels support object-level grouping
- **WHEN** a source's `grouping.signatureLabels` is `["namespace", "kind", "name"]`
- **THEN** distinct failing objects produce distinct conversations while different reasons on one object share a conversation

#### Scenario: Labels support workload-level grouping
- **WHEN** a source's `grouping.signatureLabels` is `["namespace", "workload"]` and pods of one Deployment fail across several rollouts
- **THEN** every failure carries the same `workload` label and shares one conversation, which persists across rollouts because the label does not change with pod names

#### Scenario: Workload comes from owner references
- **WHEN** an event's involved object is a pod named `app-0` owned by a StatefulSet, or `app-xk2p9` owned by a DaemonSet
- **THEN** `workload` names the owning controller in both cases, with no dependence on the shape of the pod name

#### Scenario: An unowned pod is its own workload
- **WHEN** an event's involved object is a bare pod with no owner references
- **THEN** `workload` names that pod

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

