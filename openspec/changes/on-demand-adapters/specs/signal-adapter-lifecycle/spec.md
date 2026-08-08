## MODIFIED Requirements

### Requirement: Reconciler owns the workload with the channel-adapter security posture
The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false` (unless `kubernetesAccess` mounts the token and injects `POD_NAMESPACE`); `MANAGER_URL`, **`ADAPTER_NAME` (the CR's name — the same env name channel adapters receive, replacing `SOURCE_TYPE`)**, and the per-adapter derived token injected; **`Recreate` strategy when singleton**; each served SignalSource's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<SOURCE>_`; source changes roll the pod. When `spec.port` is set the reconciler SHALL also own a Service `agentops-signal-<name>` targeting that port and inject `LISTEN_ADDR=:<port>`. Deleting the SignalAdapter SHALL remove workload and Service. SignalSources select the adapter by `spec.adapter` equalling the CR's name.

The replica count SHALL follow demand rather than being fixed at one: the Deployment runs at **one replica while at least one served `SignalSource` is claimed by a Ready `Pipeline`**, and is scaled to **zero** otherwise. An unclaimed source's signals are dropped before cooldown, so a sleeping adapter and a running one are observably identical for it. Scaling to zero SHALL leave the Deployment, ServiceAccount, owned Service, ownership, and credential projection in place, so a port-declaring adapter keeps a resolvable Service with no endpoints.

#### Scenario: Deployment shape matches the channel-adapter posture
- **WHEN** a singleton SignalAdapter's Deployment is rendered for two credentialed, claimed sources
- **THEN** it runs replicas 1/Recreate under a zero-RBAC SA without SA token automount, carries both projected credential prefixes, and its `ADAPTER_TOKEN` is the signal-context derived token

#### Scenario: Port-declaring adapter gets its Service
- **WHEN** a SignalAdapter with `port: 8080` is reconciled
- **THEN** a Service `agentops-signal-<name>` targeting 8080 exists, owned by the adapter, and the pod runs with `LISTEN_ADDR=:8080`

#### Scenario: Portless adapter gets no Service
- **WHEN** a SignalAdapter without `port` is reconciled (e.g. cron)
- **THEN** no Service is rendered

#### Scenario: kubernetesAccess mounts the token but grants nothing
- **WHEN** a SignalAdapter with `kubernetesAccess: true` is reconciled
- **THEN** its pod template automounts the SA token and carries `POD_NAMESPACE`, while the SA still has zero operator-created RBAC

#### Scenario: Default posture unchanged
- **WHEN** a SignalAdapter without `kubernetesAccess` is reconciled
- **THEN** `automountServiceAccountToken` stays false and no `POD_NAMESPACE` is injected

#### Scenario: Contract env carries the adapter's identity
- **WHEN** a SignalAdapter named `cron` is reconciled
- **THEN** its pod runs with `ADAPTER_NAME=cron` and no `SOURCE_TYPE`

#### Scenario: Unclaimed sources mean no running pod
- **WHEN** a SignalAdapter serves sources that no Ready Pipeline claims
- **THEN** its Deployment is scaled to zero, its Service still resolves with no endpoints, and the CR reports `Active=False` with reason `NoWiredSources` naming those sources

#### Scenario: Bundle enabled without a source costs nothing
- **WHEN** the vm-bundle renders its alertmanager SignalAdapter with `defaultSource.enabled=false`
- **THEN** no adapter pod runs
