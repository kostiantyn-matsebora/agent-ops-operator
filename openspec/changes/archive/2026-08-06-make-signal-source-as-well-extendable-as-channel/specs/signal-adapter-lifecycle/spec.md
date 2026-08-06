# signal-adapter-lifecycle

## ADDED Requirements

### Requirement: SignalAdapter CRD declares an implementation, never credentials
The `SignalAdapter` CRD SHALL mirror `ChannelAdapter`: required immutable `spec.type` (the signal type served), required `spec.image`, non-secret `spec.env` (Secret-referencing entries rejected with a condition), `spec.singleton` defaulting true, `spec.resources`; status with `Deployed`/`Ready`/`TypeConflict` conditions and a served-sources count. At most one active SignalAdapter per type; the newer claimant of a taken type reports `TypeConflict` and is not deployed.

#### Scenario: Plug a new signal kind with CRs only
- **WHEN** a `SignalAdapter` naming an image for `type: cron` is applied, followed by a `SignalSource` with `spec.type: cron`
- **THEN** the adapter workload is created and the source is served without any operator, chart, or helm change

#### Scenario: Duplicate type claim refused
- **WHEN** a second SignalAdapter claims an already-served type
- **THEN** the newer one reports `TypeConflict` and no second workload is created

### Requirement: Reconciler owns the workload with the channel-adapter security posture
The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false`; `MANAGER_URL`, `SOURCE_TYPE`, and the per-adapter derived token injected; `replicas 1 + Recreate` when singleton; each served SignalSource's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<SOURCE>_` (collisions after sanitization reported, never silently overwritten); source add/remove/credential changes roll the pod. Deleting the SignalAdapter SHALL remove the workload. The workload-rendering machinery SHALL be shared with the ChannelAdapter reconciler, whose rendered output MUST remain byte-identical (existing tests pin it).

#### Scenario: Deployment shape matches the channel-adapter posture
- **WHEN** a singleton SignalAdapter's Deployment is rendered for two credentialed sources
- **THEN** it runs replicas 1/Recreate under a zero-RBAC SA without SA token automount, carries both projected credential prefixes, and its `ADAPTER_TOKEN` is the signal-context derived token

#### Scenario: ChannelAdapter rendering unchanged by the refactor
- **WHEN** the existing ChannelAdapter envtest suite runs after the shared-helper extraction
- **THEN** it passes without modification

### Requirement: Unserved signal types are visible
A `SignalSource` whose `spec.type` is neither built-in (`alertmanagerWebhook`) nor claimed by a Ready `SignalAdapter` (nor adapter-reported Ready) SHALL carry `Served=False`; it SHALL flip True when a serving implementation appears — same reason vocabulary as Channel's Served condition.

#### Scenario: Typo'd type is diagnosable
- **WHEN** a SignalSource is created with `type: pagerdutty` and nothing serves it
- **THEN** the source shows `Served=False` instead of silently never producing conversations

#### Scenario: Built-in type is always served
- **WHEN** a SignalSource has `type: alertmanagerWebhook`
- **THEN** it shows `Served=True` with the in-process reason
