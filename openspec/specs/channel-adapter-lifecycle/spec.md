# channel-adapter-lifecycle

## Purpose

The ChannelAdapter CRD and its reconciler: declaring a channel-type implementation as a CR whose adapter workload the reconciler owns, with per-adapter derived auth, kubelet-only credential projection from Channels, and served/not-served visibility on both sides.
## Requirements
### Requirement: ChannelAdapter CRD declares an implementation, never credentials
The `ChannelAdapter` CRD SHALL declare a channel-type implementation as pure implementation: required `spec.image` plus workload-run properties (`singleton` defaulting true, `resources`). It SHALL carry **no configuration surface**: there is no `env` field and no credentials — per-surface settings live on the served Channels (`config`, `credentialsSecretRef`). **The CR name IS the routing key**: Channels whose `spec.adapter` equals the adapter's name are served by it; one adapter per implementation holds by construction (Kubernetes name uniqueness), with no conflict machinery.

#### Scenario: Plug a new channel type with CRs only
- **WHEN** a `ChannelAdapter` named `slack` naming an image is applied, followed by a `Channel` with `spec.adapter: slack`
- **THEN** the adapter workload is created and serves the channel without any operator, chart, or helm change

#### Scenario: No configuration on the implementation
- **WHEN** a `ChannelAdapter` manifest carries an `env` or `type` field against the new CRD
- **THEN** the fields are rejected/pruned — configuration belongs on Channels, and the name already identifies what the adapter serves

#### Scenario: Duplicate implementation impossible by construction
- **WHEN** a second `ChannelAdapter` with the same name is applied
- **THEN** the API server rejects it as an ordinary name conflict — no TypeConflict condition exists

### Requirement: Reconciler owns the adapter workload
The ChannelAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false`; `MANAGER_URL`, **`ADAPTER_NAME` (the CR's name — one env name across both adapter surfaces, replacing `CHANNEL_TYPE`)**, and the per-adapter derived token injected; `replicas 1 + Recreate` when singleton; each served Channel's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<CHANNEL>_` (collisions reported, never silently overwritten); channel add/remove/credential changes roll the pod. Deleting the ChannelAdapter SHALL remove the workload. Channels select the adapter by `spec.adapter` equalling the CR's name.

#### Scenario: Singleton discipline enforced
- **WHEN** a singleton ChannelAdapter's Deployment is rendered
- **THEN** it has replicas 1 and Recreate strategy, so no rollout ever runs two instances side by side

#### Scenario: Zero ambient authority
- **WHEN** the adapter pod starts
- **THEN** it has no ServiceAccount token mounted and its SA has no RBAC bindings

#### Scenario: Contract env carries the adapter's identity
- **WHEN** a ChannelAdapter named `telegram` is reconciled
- **THEN** its pod runs with `ADAPTER_NAME=telegram` and no `CHANNEL_TYPE`

### Requirement: Channel credentials are projected into the adapter pod
For every `Channel` of the adapter's type with `credentialsSecretRef`, the reconciler SHALL project the secret into the adapter pod spec as `envFrom` under the deterministic prefix `AGENTOPS_CRED_<CHANNEL>_` (every key `K` becomes env `<prefix>K`), resolved by the kubelet — no component reads secret values or key names through the API. Channel additions, removals, or credential changes SHALL roll the adapter pod. Sanitized-name collisions SHALL be reported as a condition, never silently overwritten.

#### Scenario: Multi-bot through one adapter
- **WHEN** two `adapter: telegram` Channels reference different bot-token Secrets
- **THEN** the single adapter pod carries both projected credentials and can serve both bots

#### Scenario: Credential change rolls the pod
- **WHEN** a Channel's `credentialsSecretRef` is changed to another Secret
- **THEN** the adapter Deployment's pod template changes and the pod restarts with the new projection

### Requirement: Unserved channel types are visible
A `Channel` whose `spec.adapter` has neither an in-process provider nor a Ready `ChannelAdapter` SHALL carry a `Served=False` condition; it SHALL flip to True when an adapter becomes ready. `ChannelAdapter.status` SHALL report deployment state and the adapter's contract-reported readiness.

#### Scenario: Typo'd type is diagnosable
- **WHEN** a Channel is created with `adapter: slak` and nothing serves that type
- **THEN** the Channel shows `Served=False` instead of ops queueing silently forever

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

