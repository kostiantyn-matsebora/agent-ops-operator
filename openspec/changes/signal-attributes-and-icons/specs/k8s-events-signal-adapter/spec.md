## MODIFIED Requirements

### Requirement: Events normalize with stable fingerprints for manager-side grouping
The adapter SHALL emit `kind: alert` signals with a deterministic fingerprint `<source>@<namespace>/<involvedObject.kind>/<involvedObject.name>/<reason>` (stable across restarts and event-object recreation).

- Labels SHALL cover at least `alertgroup: k8s-events`, `alertname: <reason>`, `namespace`, `kind`, `name`, `severity`, `workload`, and `node`, so `SignalSource.spec.grouping` can group by workload, object, reason, or namespace.
- The signal's `severity` FIELD SHALL be mapped from the Event type: `Warning` to `warning`, `Normal` to `info`. The `severity` label keeps the Event's own type for grouping.
- The payload SHALL NOT repeat the severity. The field carries it.
- `type` SHALL be a reserved key: a pod label named `type` SHALL NOT reach the signal's labels.

The `workload` label SHALL be resolved through owner references, never by parsing the object's name.

- The chain is Pod → ReplicaSet → Deployment, and the equivalent for other controllers.
- Name-based inference breaks on StatefulSets, DaemonSets, and bare pods.
- When no controller owns the object, `workload` SHALL name the object itself.

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

#### Scenario: A Warning event is rated warning
- **WHEN** a `Warning` event is normalized
- **THEN** the signal carries `severity: warning` as its field, and the payload has no `severity` key

#### Scenario: A pod label named type is dropped
- **WHEN** the involved pod carries the label `type: batch`
- **THEN** the signal's labels carry no `type`
