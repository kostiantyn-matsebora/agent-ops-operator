# channel-adapter-lifecycle (delta)

## ADDED Requirements

### Requirement: ChannelAdapter kubernetesAccess mounts API identity only
`ChannelAdapter.spec` SHALL support `kubernetesAccess` with the same semantics as `SignalAdapter.spec.kubernetesAccess`: when true, the reconciler mounts the pod's ServiceAccount token (overriding the default `automountServiceAccountToken: false`) and injects `POD_NAMESPACE`. The field grants identity only — the reconciler SHALL still create and bind no RBAC, so what the SA may do remains an external (chart/user) grant against SA `agentops-adapter-<name>` (the channel-adapter workload name; `agentops-channel-<name>` does not exist).

#### Scenario: kubernetesAccess mounts the token
- **WHEN** a ChannelAdapter with `kubernetesAccess: true` is reconciled
- **THEN** its pod has the SA token mounted and `POD_NAMESPACE` set, and no RBAC object is created or bound by the operator

#### Scenario: Default posture unchanged
- **WHEN** a ChannelAdapter omits `kubernetesAccess`
- **THEN** its pod runs with `automountServiceAccountToken: false` exactly as before this change

### Requirement: ChannelAdapter port yields a reconciler-owned Service
`ChannelAdapter.spec` SHALL support `port` with the same semantics as `SignalAdapter.spec.port`: when set, the reconciler owns a Service named after the adapter's workload — `agentops-adapter-<name>` — targeting the adapter pod on that port and injects `LISTEN_ADDR` into the pod, charts shipping no adapter connectivity. Clearing the field SHALL remove the Service. (This requirement already holds in the implementation; it is restated here because the console depends on it.)

#### Scenario: Port creates the Service
- **WHEN** a ChannelAdapter named `console` sets `port: 8080`
- **THEN** Service `agentops-adapter-console` exists targeting the pod's port 8080 and the pod runs with `LISTEN_ADDR` injected

#### Scenario: Removing the port removes the Service
- **WHEN** `port` is removed from the ChannelAdapter spec
- **THEN** the reconciler deletes the owned Service
