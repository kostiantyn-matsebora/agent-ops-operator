# k8s-bundle

## Purpose

The Kubernetes agent Helm subchart composition at `chart/charts/k8s-bundle/`: packages the k8s events signal source, the `k8s-engineer` profile with its runtime and ServiceAccount, and the RBAC granting that agent its in-cluster reach, as three individually toggleable components. Self-gated and off by default, it is also what demo mode turns on — demo mode is an enablement path for the bundle's read-only defaults, not a distinct feature set.
## Requirements
### Requirement: The k8s bundle ships as a self-gated subchart, off by default and on in demo mode
A Helm subchart at `chart/charts/k8s-bundle/` SHALL package the Kubernetes agent experience as three components — events signal source, k8s-engineer profile, and its RBAC. Every bundle template SHALL gate on `enabled OR global.demo.enabled` (self-gating, not a Helm `condition:`), with `k8s-bundle.enabled` defaulting to `false`. The parent chart's demo toggle SHALL move to `global.demo.enabled` (**BREAKING** rename from `demo.enabled`) and `chart/templates/demo.yaml` SHALL be removed — demo mode means exactly "the bundle with its defaults", which includes read-only RBAC. Explicit `k8s-bundle.*` values SHALL still apply when enabled via demo.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, AgentProfile, AgentRuntime, ServiceAccount, Pipeline, or RBAC object from the bundle is rendered

#### Scenario: Demo mode enables the bundle read-only
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** all three components render with read-only RBAC, an AgentRuntime named `default` with the configured LLM credential env, and the bundle's addressable Pipeline is askable via `POST /task` — a task names that Pipeline, never the profile

#### Scenario: Bundle without demo
- **WHEN** the chart is installed with `k8s-bundle.enabled=true` and `global.demo.enabled=false`
- **THEN** the same components render — demo mode is an enablement path, not a distinct feature set

### Requirement: Each component is individually toggleable
Within an active bundle, `eventsAdapter.enabled`, `profile.enabled`, `rbac.enabled`, and `mcp.enabled` SHALL independently control their component's objects. `eventsAdapter`, `profile`, and `rbac` default `true`. `mcp` defaults **`false`**, and the `mcpServers` sub-component that deploys the MCP server workload defaults to `false` as well: with no server workload there is no endpoint to default the config's URL onto, so a default-on MCP component would hit its own "missing endpoint fails loudly" guard on every default render — including demo mode, which is this bundle with its defaults.

Cross-component references SHALL be values-resolvable so partial enablement works. The bundle SHALL render no `Pipeline` of its own: wiring names sources and channels that come from other bundles, so it is declared by the install, not by any component of it.

#### Scenario: Events-only bundle
- **WHEN** the bundle is enabled with `profile.enabled=false` and `rbac.enabled=false`
- **THEN** only the SignalAdapter, its RBAC, and the SignalSource render, and the install claims that source from its own wiring

#### Scenario: Profile-only bundle
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the profile, runtime, SA, and RBAC render and the agent is usable through any Pipeline naming it

#### Scenario: MCP tooling without the events lane
- **WHEN** the bundle is enabled with `mcp.enabled=true` and `eventsAdapter.enabled=false`
- **THEN** the `MCPConfig` and toolsets render for operators to bind from their own Pipelines

### Requirement: The events component packages the adapter with its access
When active, the `eventsAdapter` component SHALL render: the `SignalAdapter` CR (default name `k8s-events` — the routing key SignalSources select with `spec.adapter` — values-configured image, `kubernetesAccess: true`, singleton); RBAC granting `events` `get`/`list`/`watch` AND `pods`/`replicasets` `list`/`watch` to the adapter's deterministic ServiceAccount `agentops-signal-<name>` (ClusterRole by default, namespaced Role via `rbac.clusterWide: false`, none via `rbac.create: false`); and, when `source.create` is on, a `SignalSource` naming that adapter with `severities` defaulting to `["Warning"]` and values-configurable `namespaces` and `grouping`, TOGETHER WITH the `Pipeline` claiming it. That Pipeline SHALL declare its capabilities explicitly: there is no default to inherit, so a Pipeline declaring none would hand every event-driven conversation an empty allowlist. The manager SHALL NOT create or require any RBAC verbs on roles or rolebindings.

The pods/replicasets grant is read-only and exists because the adapter resolves workload identity through owner references and re-checks liveness before emitting. Where the events grant is namespaced, the pods/replicasets grant SHALL be namespaced identically — the adapter never reads more broadly than it watches.

The rendered source's default `grouping.signatureLabels` SHALL be `["namespace", "workload"]`, and its default `rules` SHALL be calibrated against both failure modes at once: they SHALL NOT open a conversation for ordinary rollout churn, and they SHALL NOT lose an actionable incident. Specifically the shipped defaults SHALL drop only pure-bookkeeping reasons whose underlying problem another undropped reason still reports, SHALL assign `for: 0` to reasons describing a completed event and to node-level conditions, SHALL assign longer dwells with breadth escalation to the known-flappy reasons, and SHALL end in a catch-all dwell so unanticipated reasons are verified rather than discarded.

#### Scenario: A healthy rollout produces no conversation
- **WHEN** the bundle renders with default values and a ten-replica Deployment rolls out normally, emitting probe and scheduling warnings on pods that then become Ready or terminate
- **THEN** no conversation is created

#### Scenario: A broken rollout produces exactly one conversation
- **WHEN** the same Deployment is rolled out with an unpullable image
- **THEN** exactly one conversation is created for the workload, carrying every contributing reason with its occurrence count

#### Scenario: One values flag yields flowing events
- **WHEN** the bundle is enabled with defaults and the LLM credential Secret exists
- **THEN** Warning events in the cluster produce conversations executed by the k8s-engineer profile without building images or applying extra manifests

#### Scenario: The rendered source is always claimed
- **WHEN** the events component renders a SignalSource
- **THEN** a Pipeline referencing that source renders alongside it, so signals route instead of dropping with `Wired=False`

#### Scenario: Event-driven conversations are equipped
- **WHEN** an event routes through the bundle's rendered Pipeline
- **THEN** the resulting work unit carries a non-empty allowlist, because that Pipeline declares its own toolsets

#### Scenario: Namespace-scoped events RBAC
- **WHEN** `eventsAdapter.rbac.clusterWide=false`
- **THEN** only a namespaced Role/RoleBinding renders, covering events, pods and replicasets alike, and the adapter can watch only in the release namespace

#### Scenario: A default install groups by workload
- **WHEN** the bundle renders with default values and a ten-replica Deployment crash-loops through several rollouts
- **THEN** the rendered source's `signatureLabels` are `["namespace", "workload"]` and one conversation covers the workload rather than one per pod

#### Scenario: Default rules ship, not an empty filter
- **WHEN** the bundle renders with default values
- **THEN** the rendered source carries a non-empty `rules` list, so rollout churn is suppressed without the installer writing any configuration

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render: the `k8s-engineer` AgentProfile (values-configurable name, `maxTurns`, no repository, and **no capabilities** — no `allowedTools`, no `mcp`); a dedicated runtime ServiceAccount (default `agentops-runtime-k8s`); and, when `runtime.create` is on, an `AgentRuntime` (values-configured image, optional `nodeSelector`, and an LLM credential Secret ref projected as env via `valueFrom` — the manager reads no Secrets) whose `serviceAccountName` is that SA.

Because the profile has NO repository, no agent definition file can be resolved for it. The component SHALL therefore support an inline role (`systemPrompt`) and ship a sensible default, so the shipped agent is not personality-free: a conversation woken by a cluster event would otherwise arrive with no instructions at all.

`runtime.create: false` SHALL support operators wiring the profile to an existing runtime via `runtimeRef` values.

#### Scenario: Profile executes under the bundle SA
- **WHEN** the bundle renders with defaults and a task reaches `k8s-engineer`
- **THEN** the conversation's runtime pod runs under the bundle's ServiceAccount, so the agent's in-cluster power is exactly what the bundle's RBAC component granted

#### Scenario: Bring-your-own runtime
- **WHEN** `profile.runtime.create=false` and `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the AgentProfile renders with that `runtimeRef` and no AgentRuntime or SA is created by the bundle

#### Scenario: The profile stays free of capabilities
- **WHEN** the bundle renders with the MCP component active
- **THEN** the `MCPConfig` is referenced by the install's wiring and the AgentProfile itself declares no `mcp` block and no tools, so profiles stay reusable across differently-tooled routes

#### Scenario: The demo addresses a Pipeline
- **WHEN** a task is posted naming the install's Pipeline for this profile
- **THEN** the work unit carries that Pipeline's tools, and the rendered AgentProfile declares none — the bundle ships no Pipeline of its own, so the route is the one the install declared

#### Scenario: An observe-only agent
- **WHEN** the install's Pipeline binds the observation toolset without the shell or mutating ones
- **THEN** the agent reads the cluster but changes nothing, because the allowlist is the whole grant

#### Scenario: The repo-less agent still has a role
- **WHEN** the bundle renders with defaults
- **THEN** the AgentProfile carries an inline role describing the agent's job and how to act on a cluster, which the runtime appends to its system prompt

### Requirement: RBAC is read-only by default with an explicit full mode
When active, the `rbac` component SHALL bind roles to the profile's runtime ServiceAccount according to `rbac.mode`: `readonly` (default) binds the built-in `view` ClusterRole plus a bundle ClusterRole granting `get`/`list`/`watch` on `nodes` and `namespaces` and `get`/`list` on `metrics.k8s.io` nodes/pods (the pre-bundle demo grants, verbatim); `full` binds the built-in `cluster-admin` ClusterRole. `mode: full` SHALL never be a default anywhere (including demo mode) and SHALL be documented as granting the agent unrestricted cluster control. `rbac.enabled: false` SHALL render no bindings, leaving the SA powerless.

#### Scenario: Readonly is the default everywhere
- **WHEN** the bundle is enabled (directly or via demo) without setting `rbac.mode`
- **THEN** only the `view` binding and the read-only ClusterRole render; no write verb is granted anywhere

#### Scenario: Full mode is an explicit opt-in
- **WHEN** `k8s-bundle.rbac.mode=full` is set
- **THEN** a ClusterRoleBinding to `cluster-admin` for the runtime SA renders in place of the read-only objects

#### Scenario: RBAC off means a powerless agent
- **WHEN** `rbac.enabled=false`
- **THEN** no bindings render and the k8s-engineer agent cannot read the cluster API

### Requirement: Demo values migrate to bundle paths
The pre-bundle demo values SHALL move: `demo.enabled` → `global.demo.enabled`, `demo.runtimeImage` → `k8s-bundle.profile.runtime.image`, `demo.credentialsSecret.*` → `k8s-bundle.profile.runtime.credentialsSecret.*`, `demo.readOnlyRbac` → `k8s-bundle.rbac.mode` (true ≙ `readonly`). The chart major version SHALL be bumped and the README SHALL carry the migration table. Upgrading a demo-enabled release SHALL preserve semantics: the AgentRuntime `default` re-renders equivalently (existing conversations keep resolving their runtime) while demo-named SA/RBAC objects are replaced by bundle-named ones.

#### Scenario: Upgraded demo release keeps working
- **WHEN** a release running chart 1.x with `demo.enabled=true` upgrades and adopts the new values paths
- **THEN** the demo advisor flow works — now addressed as `POST /task` naming the bundle's Pipeline — and old demo-suffixed SA/RBAC objects are removed by the upgrade

