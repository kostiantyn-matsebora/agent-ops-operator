# channel-adapter-lifecycle — delta

## MODIFIED Requirements

### Requirement: Reconciler owns the adapter workload
The ChannelAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false`; `MANAGER_URL`, **`ADAPTER_NAME` (the CR's name — one env name across both adapter surfaces, replacing `CHANNEL_TYPE`)**, and the per-adapter derived token injected; `replicas 1 + Recreate` when singleton; each served Channel's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<CHANNEL>_` (collisions reported, never silently overwritten); channel add/remove/credential changes roll the pod. Deleting the ChannelAdapter SHALL remove the workload. Channels select the adapter by `spec.adapter` equalling the CR's name.

#### Scenario: Contract env carries the adapter's identity
- **WHEN** a ChannelAdapter named `telegram` is reconciled
- **THEN** its pod runs with `ADAPTER_NAME=telegram` and no `CHANNEL_TYPE`
