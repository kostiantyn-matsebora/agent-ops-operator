# signal-adapter-lifecycle — delta

## MODIFIED Requirements

### Requirement: Reconciler owns the workload with the channel-adapter security posture
The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no manager-granted RBAC and `automountServiceAccountToken: false` by default; `MANAGER_URL`, `SOURCE_TYPE`, and the per-adapter derived token injected; `replicas 1 + Recreate` when singleton; each served SignalSource's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<SOURCE>_` (collisions after sanitization reported, never silently overwritten); source add/remove/credential changes roll the pod. A SignalAdapter whose data source is the Kubernetes API itself MAY opt in via `spec.automountServiceAccountToken: true` (default false), which mounts the pod's own SA token and nothing more — the reconciler SHALL NOT create Roles, ClusterRoles, or bindings, so the token confers only whatever RBAC the chart or cluster operator has bound to the deterministic SA name out-of-band. Deleting the SignalAdapter SHALL remove the workload. The workload-rendering machinery SHALL be shared with the ChannelAdapter reconciler, whose rendered output MUST remain byte-identical (existing tests pin it); `ChannelAdapter` SHALL NOT expose the automount opt-in.

#### Scenario: Deployment shape matches the channel-adapter posture
- **WHEN** a singleton SignalAdapter's Deployment is rendered for two credentialed sources
- **THEN** it runs replicas 1/Recreate under a zero-RBAC SA without SA token automount, carries both projected credential prefixes, and its `ADAPTER_TOKEN` is the signal-context derived token

#### Scenario: Automount opt-in mounts the token and nothing more
- **WHEN** a SignalAdapter sets `spec.automountServiceAccountToken: true`
- **THEN** the rendered pod spec has `automountServiceAccountToken: true` with the same dedicated SA, and the reconciler creates no RBAC objects

#### Scenario: Default posture is unchanged
- **WHEN** a SignalAdapter omits `spec.automountServiceAccountToken`
- **THEN** the rendered pod spec keeps `automountServiceAccountToken: false`, identical to the pre-change output

#### Scenario: ChannelAdapter rendering unchanged by the refactor
- **WHEN** the existing ChannelAdapter envtest suite runs after the shared-helper extraction
- **THEN** it passes without modification
