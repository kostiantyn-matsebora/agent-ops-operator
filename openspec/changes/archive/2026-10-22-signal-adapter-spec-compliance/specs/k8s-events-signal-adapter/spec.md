# k8s-events-signal-adapter delta

## MODIFIED Requirements

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
rather than emit under the object's own name.

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

#### Scenario: An unowned pod is its own workload
- **WHEN** an event's involved object is a bare pod with no owner references
- **THEN** the id keys on that pod

#### Scenario: Distinct workloads never merge
- **WHEN** two unrelated workloads fail in one namespace
- **THEN** their signals carry distinct `initiationId` values and open distinct conversations
