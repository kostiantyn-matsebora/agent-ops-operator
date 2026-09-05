## ADDED Requirements

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
