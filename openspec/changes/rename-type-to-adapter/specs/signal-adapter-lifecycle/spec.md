# signal-adapter-lifecycle — delta

## MODIFIED Requirements

### Requirement: Reconciler owns the workload with the channel-adapter security posture
The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false` (unless `kubernetesAccess` mounts the token and injects `POD_NAMESPACE`); `MANAGER_URL`, **`ADAPTER_NAME` (the CR's name — the same env name channel adapters receive, replacing `SOURCE_TYPE`)**, and the per-adapter derived token injected; `replicas 1 + Recreate` when singleton; each served SignalSource's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<SOURCE>_`; source changes roll the pod. When `spec.port` is set the reconciler SHALL also own a Service `agentops-signal-<name>` targeting that port and inject `LISTEN_ADDR=:<port>`. Deleting the SignalAdapter SHALL remove workload and Service. SignalSources select the adapter by `spec.adapter` equalling the CR's name.

#### Scenario: Contract env carries the adapter's identity
- **WHEN** a SignalAdapter named `cron` is reconciled
- **THEN** its pod runs with `ADAPTER_NAME=cron` and no `SOURCE_TYPE`
