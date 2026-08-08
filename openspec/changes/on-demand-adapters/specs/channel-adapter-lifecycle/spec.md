## MODIFIED Requirements

### Requirement: Reconciler owns the adapter workload
The ChannelAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false`; `MANAGER_URL`, **`ADAPTER_NAME` (the CR's name — one env name across both adapter surfaces, replacing `CHANNEL_TYPE`)**, and the per-adapter derived token injected; **`Recreate` strategy when singleton**; each served Channel's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<CHANNEL>_` (collisions reported, never silently overwritten); channel add/remove/credential changes roll the pod. Deleting the ChannelAdapter SHALL remove the workload. Channels select the adapter by `spec.adapter` equalling the CR's name.

The replica count SHALL follow demand rather than being fixed at one: the Deployment runs at **one replica while at least one `Channel` names the adapter**, and is scaled to **zero** otherwise. A Channel existing is sufficient demand — a Pipeline reference SHALL NOT be required — because a polling channel adapter at zero replicas cannot receive, and nothing could wake it. Scaling to zero SHALL leave the Deployment, ServiceAccount, ownership, and credential projection in place.

#### Scenario: Singleton discipline enforced
- **WHEN** a singleton ChannelAdapter's Deployment is rendered with demand
- **THEN** it has replicas 1 and Recreate strategy, so no rollout ever runs two instances side by side

#### Scenario: Zero ambient authority
- **WHEN** the adapter pod starts
- **THEN** it has no ServiceAccount token mounted and its SA has no RBAC bindings

#### Scenario: Contract env carries the adapter's identity
- **WHEN** a ChannelAdapter named `telegram` is reconciled
- **THEN** its pod runs with `ADAPTER_NAME=telegram` and no `CHANNEL_TYPE`

#### Scenario: No served channels means no running pod
- **WHEN** a ChannelAdapter exists and no Channel names it
- **THEN** its Deployment is present but scaled to zero, and the CR reports `Active=False`

#### Scenario: A channel alone is enough demand
- **WHEN** a Channel names the adapter and no Pipeline references that Channel
- **THEN** the Deployment runs at one replica
