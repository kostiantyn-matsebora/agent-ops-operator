# console-deployment

## ADDED Requirements

### Requirement: Console ships as an opt-in chart bundle of CRs
The Helm chart SHALL package the console behind `console.enabled` (default **false**): a `ChannelAdapter` named `console` (image, `singleton: true`, `kubernetesAccess: true`, `port` set), a `Channel` with `spec.adapter: console` referencing the UI token Secret via `credentialsSecretRef`, and the token Secret itself. The adapter workload, Service, token injection, and credential projection SHALL all come from the reconcilers — the chart ships no Deployment or Service for the console.

#### Scenario: Enabling the console is CRs-only
- **WHEN** `console.enabled=true` is set and the release upgraded
- **THEN** the ChannelAdapter/Channel/Secret are applied and the reconcilers bring up the console workload and Service with no chart-owned workload objects

#### Scenario: Disabling is non-destructive to conversations
- **WHEN** `console.enabled` is flipped to false
- **THEN** the console workload and Service are removed, referencing Channels report `Served=False`, and existing Conversations keep their other channel threads

### Requirement: Read-only RBAC granted by the chart, not the operator
The chart SHALL grant SA `agentops-channel-console` a namespaced Role with only `get`/`list`/`watch` on `agentops.dev` resources (no Secrets, no core resources, no write verbs), bound only when `console.enabled=true`. No reconciler SHALL create or bind any RBAC for the console.

#### Scenario: Console SA can watch, not write
- **WHEN** access for SA `agentops-channel-console` is checked
- **THEN** `list`/`watch` on pipelines/conversations succeed and any write verb or Secret read is denied

### Requirement: Browser access is authenticated
The console UI and its APIs SHALL require the chart-provisioned bearer token (projected to the adapter via the console Channel's `credentialsSecretRef`), validated with constant-time comparison; unauthenticated requests receive 401. The Service SHALL default to ClusterIP with an optional Ingress template.

#### Scenario: No anonymous access
- **WHEN** a browser requests any console page or API without the token (or session established from it)
- **THEN** the console responds 401 / login prompt and serves no topology, CR, or conversation data

### Requirement: Wiring the console into pipelines is explicit
Joining the console to a pipeline SHALL use only the existing mechanism — adding the console Channel to `Pipeline.spec.channels[]` — performed by the user (or their own automation), never by the chart or a reconciler mutating Pipelines. The console UI SHALL surface which pipelines are not joined and show the exact edit required.

#### Scenario: Unjoined pipeline shows join instructions
- **WHEN** a user views a pipeline whose channels do not include the console Channel
- **THEN** the console displays the `channels[]` addition needed to join it, and performs no mutation itself
