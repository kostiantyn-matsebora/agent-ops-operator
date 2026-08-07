# signal-adapter-lifecycle

## Purpose

The SignalAdapter CRD and its reconciler: declaring a signal-type implementation as a CR whose adapter workload the reconciler owns with the channel-adapter security posture, per-type conflict guarding, credential projection from served sources, and Served visibility on SignalSources.

## Requirements

### Requirement: SignalAdapter CRD declares an implementation, never credentials
The `SignalAdapter` CRD SHALL mirror `ChannelAdapter` as pure implementation: required `spec.image`, workload-run properties (`singleton` defaulting true, `resources`), an optional `spec.port` declaring the port the image's own HTTP surface listens on (webhook-receiving implementations), and an optional `spec.kubernetesAccess` (default false) declaring that the implementation talks to the Kubernetes API. It SHALL carry no `env`, no `type`, and no credentials. **The CR name IS the routing key**: SignalSources whose `spec.adapter` equals the adapter's name are served by it; one adapter per implementation holds by construction.

#### Scenario: Plug a new signal kind with CRs only
- **WHEN** a `SignalAdapter` named `cron` naming an image is applied, followed by a `SignalSource` with `spec.adapter: cron`
- **THEN** the adapter workload is created and the source is served without any operator, chart, or helm change

#### Scenario: Duplicate implementation impossible by construction
- **WHEN** a second `SignalAdapter` with the same name is applied
- **THEN** the API server rejects it as an ordinary name conflict — no TypeConflict condition exists

### Requirement: Reconciler owns the workload with the channel-adapter security posture
The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false` (unless `kubernetesAccess` mounts the token and injects `POD_NAMESPACE`); `MANAGER_URL`, **`ADAPTER_NAME` (the CR's name — the same env name channel adapters receive, replacing `SOURCE_TYPE`)**, and the per-adapter derived token injected; `replicas 1 + Recreate` when singleton; each served SignalSource's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<SOURCE>_`; source changes roll the pod. When `spec.port` is set the reconciler SHALL also own a Service `agentops-signal-<name>` targeting that port and inject `LISTEN_ADDR=:<port>`. Deleting the SignalAdapter SHALL remove workload and Service. SignalSources select the adapter by `spec.adapter` equalling the CR's name.

#### Scenario: Deployment shape matches the channel-adapter posture
- **WHEN** a singleton SignalAdapter's Deployment is rendered for two credentialed sources
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

### Requirement: Unserved signal types are visible
A `SignalSource` whose `spec.adapter` is not claimed by a Ready `SignalAdapter` (nor adapter-reported Ready) SHALL carry `Served=False`; it SHALL flip True when a serving adapter appears — same reason vocabulary as Channel's Served condition. There are no built-in signal types: every type needs an adapter. `kubectl get signalsources` SHALL surface useful state (type, received count) without dead columns.

#### Scenario: Typo'd type is diagnosable
- **WHEN** a SignalSource is created with `adapter: pagerdutty` and nothing serves it
- **THEN** the source shows `Served=False` instead of silently never producing conversations

#### Scenario: No type is served without an adapter
- **WHEN** a SignalSource uses `type: alertmanagerWebhook` after the built-in removal and no adapter claims that type
- **THEN** it shows `Served=False` — the manager itself serves nothing
