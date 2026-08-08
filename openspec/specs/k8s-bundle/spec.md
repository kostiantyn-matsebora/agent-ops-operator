# k8s-bundle

## Purpose

The Kubernetes agent Helm subchart composition at `chart/charts/k8s-bundle/`: packages the k8s events signal source, the `k8s-engineer` profile with its runtime and ServiceAccount, and the RBAC granting that agent its in-cluster reach, as three individually toggleable components. Self-gated and off by default, it is also what demo mode turns on — demo mode is an enablement path for the bundle's read-only defaults, not a distinct feature set.

## Requirements

### Requirement: The k8s bundle ships as a self-gated subchart, off by default and on in demo mode
A Helm subchart at `chart/charts/k8s-bundle/` SHALL package the Kubernetes agent experience as three components — events signal source, k8s-engineer profile, and its RBAC. Every bundle template SHALL gate on `enabled OR global.demo.enabled` (self-gating, not a Helm `condition:`), with `k8s-bundle.enabled` defaulting to `false`. The parent chart's demo toggle SHALL move to `global.demo.enabled` (**BREAKING** rename from `demo.enabled`) and `chart/templates/demo.yaml` SHALL be removed — demo mode means exactly "the bundle with its defaults", which includes read-only RBAC. Explicit `k8s-bundle.*` values SHALL still apply when enabled via demo.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, AgentProfile, AgentRuntime, ServiceAccount, or RBAC object from the bundle is rendered

#### Scenario: Demo mode enables the bundle read-only
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** all three components render with read-only RBAC, an AgentRuntime named `default` with the configured LLM credential env, and the `k8s-engineer` profile is askable via `POST /task` exactly as the pre-bundle demo was

#### Scenario: Bundle without demo
- **WHEN** the chart is installed with `k8s-bundle.enabled=true` and `global.demo.enabled=false`
- **THEN** the same components render — demo mode is an enablement path, not a distinct feature set

### Requirement: Each component is individually toggleable
Within an active bundle, `eventsAdapter.enabled`, `profile.enabled`, and `rbac.enabled` SHALL independently control their component's objects (all default `true`). Cross-component references SHALL be values-resolvable so partial enablement works: the rendered Pipeline's `profileRef` defaults to the bundle's profile name but is overridable; RBAC binds the values-named runtime ServiceAccount; a render-time failure with a clear message SHALL occur when `eventsAdapter.source.create` is on but no profile name resolves.

#### Scenario: Events-only bundle
- **WHEN** the bundle is enabled with `profile.enabled=false`, `rbac.enabled=false`, and `eventsAdapter.source.profileRef` pointing at an operator-provided profile
- **THEN** only the SignalAdapter, its RBAC, the SignalSource, and the Pipeline claiming it render, wired to that profile

#### Scenario: Profile-only bundle
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the profile, runtime, SA, and RBAC render and the agent is usable via `/task` with no event ingestion

### Requirement: The events component packages the adapter with its access
When active, the `eventsAdapter` component SHALL render: the `SignalAdapter` CR (default name `k8s-events` — the routing key SignalSources select with `spec.adapter` — values-configured image, `kubernetesAccess: true`, singleton); RBAC granting `events` `get`/`list`/`watch` to the adapter's deterministic ServiceAccount `agentops-signal-<name>` (ClusterRole by default, namespaced Role via `rbac.clusterWide: false`, none via `rbac.create: false`); and, when `source.create` is on, a `SignalSource` naming that adapter with `severities` defaulting to `["Warning"]` and values-configurable `namespaces` and `grouping`, TOGETHER WITH the `Pipeline` claiming it. The Pipeline is not optional: wiring is pipeline-only, so an unclaimed source drops every event with `Wired=False`. The manager SHALL NOT create or require any RBAC verbs on roles or rolebindings.

#### Scenario: One values flag yields flowing events
- **WHEN** the bundle is enabled with defaults and the LLM credential Secret exists
- **THEN** Warning events in the cluster produce conversations executed by the k8s-engineer profile without building images or applying extra manifests

#### Scenario: The rendered source is always claimed
- **WHEN** the events component renders a SignalSource
- **THEN** a Pipeline referencing that source renders alongside it, so signals route instead of dropping with `Wired=False`

#### Scenario: Namespace-scoped events RBAC
- **WHEN** `eventsAdapter.rbac.clusterWide=false`
- **THEN** only a namespaced Role/RoleBinding renders and the adapter can watch events only in the release namespace

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render: the `k8s-engineer` AgentProfile (values-configurable name, `maxTurns` 40, no repository — and NO capabilities, since AgentProfiles declare none); a dedicated runtime ServiceAccount (default `agentops-runtime-k8s`); and, when `runtime.create` is on, an `AgentRuntime` (name defaulting to `default`, values-configured image and LLM credential Secret ref projected as env via `valueFrom` — the manager reads no Secrets) whose `serviceAccountName` is that SA. `runtime.create: false` SHALL support operators wiring the profile to an existing runtime via `runtimeRef` values.

When `profile.baseline.create` is on, the component SHALL also render that profile's **capability-only Pipeline** — no sources, no channels — binding the chart's built-in toolsets, with `baseline.grantShell` controlling whether execution is among them. Without it a bare `POST /task` would reach an agent with no tools at all, which is exactly what the documented five-minute demo does.

#### Scenario: Profile executes under the bundle SA
- **WHEN** the bundle renders with defaults and a task addresses `k8s-engineer`
- **THEN** the conversation's runtime pod runs under the bundle's ServiceAccount, so the agent's in-cluster power is exactly what the bundle's RBAC component granted

#### Scenario: Bring-your-own runtime
- **WHEN** `profile.runtime.create=false` and `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the AgentProfile renders with that `runtimeRef` and no AgentRuntime or SA is created by the bundle

#### Scenario: The demo stays usable without any routing wiring
- **WHEN** the bundle renders with defaults and a task is posted to `POST /task` with no pipeline named
- **THEN** the baseline Pipeline supplies the agent's tools, and the rendered AgentProfile declares none

#### Scenario: An observe-only agent
- **WHEN** `profile.baseline.grantShell=false`
- **THEN** the baseline binds the observation toolset only, so the agent reads the cluster but runs nothing through the pipeline-less paths

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
- **WHEN** a release running chart 1.x with `demo.enabled=true` upgrades to 2.x with `global.demo.enabled=true`
- **THEN** the demo advisor flow (`POST /task` with profile `k8s-engineer`) works unchanged and old demo-suffixed SA/RBAC objects are removed by the upgrade
