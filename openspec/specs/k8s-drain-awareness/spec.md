# k8s-drain-awareness

## Purpose

While a node is deliberately being taken out of service, the events its workloads emit describe the drain and not a fault; the `signals/k8s-events/` adapter suppresses them for exactly as long as the drain lasts and reports a drain that lasts too long.
## Requirements

### Requirement: A draining node is recognised by the manager's predicate
The adapter SHALL consider a node DRAINING when `spec.unschedulable` is true or
when it carries a `NoSchedule` or `NoExecute` taint whose key is not one
Kubernetes applies from node conditions (`node.kubernetes.io/not-ready`,
`unreachable`, `memory-pressure`, `disk-pressure`, `pid-pressure`,
`network-unavailable`, `out-of-service`). That set SHALL be the same one the
manager's drain awareness excludes, pinned by test on both sides.
`node.kubernetes.io/unschedulable` is deliberately NOT in that set — it IS the
cordon, the taint `kubectl cordon` adds alongside the flag, so treating it as a
mere condition would make a cordon exclude itself. A node that is merely
unwell SHALL NOT count as draining.

#### Scenario: A cordon counts
- **WHEN** a reboot manager cordons a node
- **THEN** the adapter treats it as draining from that moment

#### Scenario: NotReady alone does not count
- **WHEN** a node loses its kubelet heartbeat and gains only `not-ready` and `unreachable` taints
- **THEN** the adapter does not treat it as draining and its events follow the ordinary rules

### Requirement: Events on a draining node are suppressed for the drain's duration
While a node is draining, the adapter SHALL suppress every event whose involved
object runs on that node and every event on the Node object itself, before the
dwell queue, and SHALL stop suppressing the moment the node stops draining.
Suppression SHALL be per source, ON by default, and disabled by
`config.route.drainingNodes: report`. A `config.route.drainingNodeMatchers`
list, when present, SHALL narrow suppression to events matching it. An event
whose object's node is unknown to the cache SHALL NOT be suppressed.

#### Scenario: A rolling reboot opens nothing
- **WHEN** a node is cordoned and every pod on it emits `NodeNotReady` during its reboot
- **THEN** no signal is emitted for those events and no conversation opens

#### Scenario: Suppression ends with the uncordon
- **WHEN** the node is uncordoned and a pod on it emits `BackOff` afterwards
- **THEN** the event follows its ordinary rule

#### Scenario: Opt-out per source
- **WHEN** a source sets `route.drainingNodes: report`
- **THEN** events on draining nodes are evaluated by the rules exactly as before

### Requirement: A drain that outlives its bound is reported once and stops suppressing
`config.route.drainingNodeBound` (duration, default `1h`) SHALL bound how long a
node's drain suppresses. When a node has been draining longer, the adapter
SHALL emit ONE Node-kind signal naming the node and how long it has been
draining, and SHALL stop suppressing that node's events until it stops
draining and drains again.

#### Scenario: A forgotten cordon surfaces
- **WHEN** a node stays cordoned past the bound
- **THEN** exactly one signal for that Node is posted and the node's pod events resume ordinary evaluation

### Requirement: Drain suppression is visible on the source
While any node is draining, the source SHALL report `Ready=True` with a reason
naming the draining nodes and the number of events suppressed so far; when
the last drain ends, the reason SHALL report the total once.

#### Scenario: Suppression is not silence
- **WHEN** a node is draining and ten of its events were suppressed
- **THEN** the source's Ready condition names the node and the count

### Requirement: Without node access the feature is off and says so
When the adapter's ServiceAccount cannot list and watch nodes, drain awareness
SHALL be disabled for every source, the adapter SHALL report that once on each
source's Ready condition without setting `Ready=False`, and every other
behaviour SHALL be unchanged.

#### Scenario: A namespaced install
- **WHEN** the events component renders with `rbac.clusterWide: false`
- **THEN** sources stay `Ready=True`, their condition notes that drain awareness is unavailable, and events on cordoned nodes follow the ordinary rules
