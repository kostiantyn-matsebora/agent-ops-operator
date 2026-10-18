# k8s-events-signal-adapter — delta

## REMOVED Requirements

### Requirement: Events normalize with stable fingerprints for manager-side grouping

**Reason**: Per-event fingerprints and the `spec.grouping` labels they fed are retired. The adapter now decides correlation itself, keyed on the workload it already resolves.

**Migration**: Replaced by "Events normalize with a workload-keyed correlation id" in this change. Owner-reference workload resolution and the label set survive unchanged, configurable object-level grouping does not — one conversation per workload is the fixed key.

## ADDED Requirements

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
controllers), never by parsing the object's name. Name-based inference breaks
on StatefulSets, DaemonSets, and bare pods.

When no controller owns the object, `workload` SHALL name the object itself.

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

#### Scenario: An unowned pod is its own workload
- **WHEN** an event's involved object is a bare pod with no owner references
- **THEN** the id keys on that pod

#### Scenario: Distinct workloads never merge
- **WHEN** two unrelated workloads fail in one namespace
- **THEN** their signals carry distinct `initiationId` values and open distinct conversations
