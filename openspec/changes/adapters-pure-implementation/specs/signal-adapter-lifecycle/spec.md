# signal-adapter-lifecycle — delta

## MODIFIED Requirements

### Requirement: SignalAdapter CRD declares an implementation, never credentials
The `SignalAdapter` CRD SHALL mirror `ChannelAdapter` as pure implementation: required `spec.image`, workload-run properties (`singleton` defaulting true, `resources`), and an optional `spec.port` declaring the port the image's own HTTP surface listens on (webhook-receiving implementations). It SHALL carry no `env`, no `type`, and no credentials. **The CR name IS the routing key**: SignalSources whose `spec.type` equals the adapter's name are served by it; one adapter per implementation holds by construction.

#### Scenario: Plug a new signal kind with CRs only
- **WHEN** a `SignalAdapter` named `cron` naming an image is applied, followed by a `SignalSource` with `spec.type: cron`
- **THEN** the adapter workload is created and the source is served without any operator, chart, or helm change

#### Scenario: Duplicate implementation impossible by construction
- **WHEN** a second `SignalAdapter` with the same name is applied
- **THEN** the API server rejects it as an ordinary name conflict — no TypeConflict condition exists

### Requirement: Reconciler owns the workload with the channel-adapter security posture
The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per adapter: dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false`; `MANAGER_URL`, `SOURCE_TYPE` (= the CR name), and the per-adapter derived token injected; `replicas 1 + Recreate` when singleton; each served SignalSource's `credentialsSecretRef` projected as `envFrom` under `AGENTOPS_CRED_<SOURCE>_` (collisions reported, never silently overwritten); source changes roll the pod. **When `spec.port` is set, the reconciler SHALL also own a Service `agentops-signal-<name>` targeting that port on the deterministic pod label, and inject `LISTEN_ADDR=:<port>`** — enabling an adapter yields a complete appliance with no chart-side connectivity. Deleting the SignalAdapter SHALL remove workload and Service. The workload machinery stays shared with the ChannelAdapter reconciler.

#### Scenario: Port-declaring adapter gets its Service
- **WHEN** a SignalAdapter with `port: 8080` is reconciled
- **THEN** a Service `agentops-signal-<name>` targeting 8080 exists, owned by the adapter, and the pod runs with `LISTEN_ADDR=:8080`

#### Scenario: Portless adapter gets no Service
- **WHEN** a SignalAdapter without `port` is reconciled (e.g. cron)
- **THEN** no Service is rendered
