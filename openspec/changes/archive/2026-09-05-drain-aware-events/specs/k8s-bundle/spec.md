## ADDED Requirements

### Requirement: The events grant carries nodes cluster-wide, and tier 3 is Node-kind
When `rbac.clusterWide` is true, the events component's ClusterRole SHALL
carry `nodes` `list`/`watch` beside events, pods and replicasets. A namespaced
Role SHALL NOT carry it, because nodes are cluster-scoped. The shipped
node-condition rule (`NodeNotReady` and the pressure reasons at `for: 0`)
SHALL match `kind="Node"` only, so a pod-level `NodeNotReady` outside a drain
falls to the catch-all's dwell and liveness re-check. The rendered source's
default `route.drainingNodes` SHALL be `suppress`.

#### Scenario: Cluster-wide grant includes nodes
- **WHEN** the bundle renders with default values
- **THEN** the events ClusterRole lists `nodes` with `list` and `watch`, and the namespaced variant does not

#### Scenario: Pod-level NodeNotReady dwells
- **WHEN** a pod emits `NodeNotReady` on a node that is not draining and is Ready again within the catch-all dwell
- **THEN** no signal is emitted
