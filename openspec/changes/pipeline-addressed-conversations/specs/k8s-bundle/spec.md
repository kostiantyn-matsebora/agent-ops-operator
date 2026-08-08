# k8s-bundle — delta

## MODIFIED Requirements

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render: the `k8s-engineer` AgentProfile (values-configurable name, `maxTurns` 40, no repository, and NO capabilities); a dedicated runtime ServiceAccount (default `agentops-runtime-k8s`); and, when `runtime.create` is on, an `AgentRuntime` (name defaulting to `default`, values-configured image and LLM credential Secret ref projected as env via `valueFrom` — the manager reads no Secrets) whose `serviceAccountName` is that SA. `runtime.create: false` SHALL support operators wiring the profile to an existing runtime via `runtimeRef` values.

It SHALL also render an ADDRESSABLE Pipeline for the shipped agent, declaring the chart's built-in toolsets, so the task API has a route to name — under demo mode and behind a values flag so a production install can opt out or supply its own. This replaces the capability-only baseline Pipeline: it is an ordinary route that declares capabilities, not a per-profile default.

#### Scenario: Profile executes under the bundle SA
- **WHEN** the bundle renders with defaults and a task addresses the agent's Pipeline
- **THEN** the conversation's runtime pod runs under the bundle's ServiceAccount, so the agent's in-cluster power is exactly what the bundle's RBAC component granted

#### Scenario: Bring-your-own runtime
- **WHEN** `profile.runtime.create=false` and `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the AgentProfile renders with that `runtimeRef` and no AgentRuntime or SA is created by the bundle

#### Scenario: The demo addresses a Pipeline
- **WHEN** the bundle renders with defaults and a task is posted naming the shipped Pipeline
- **THEN** the work unit carries that Pipeline's tools, and the rendered AgentProfile declares none

#### Scenario: An observe-only agent
- **WHEN** the shipped Pipeline's shell grant is disabled in values
- **THEN** it binds the observation toolset only, so the agent reads the cluster but runs nothing

### Requirement: The events component packages the adapter with its access
When active, the `eventsAdapter` component SHALL render: the `SignalAdapter` CR (default name `k8s-events` — the routing key SignalSources select with `spec.adapter` — values-configured image, `kubernetesAccess: true`, singleton); RBAC granting `events` `get`/`list`/`watch` to the adapter's deterministic ServiceAccount `agentops-signal-<name>` (ClusterRole by default, namespaced Role via `rbac.clusterWide: false`, none via `rbac.create: false`); and, when `source.create` is on, a `SignalSource` naming that adapter with `severities` defaulting to `["Warning"]` and values-configurable `namespaces` and `grouping`, TOGETHER WITH the `Pipeline` claiming it. That Pipeline SHALL declare its capabilities explicitly: there is no default to inherit, so a Pipeline declaring none would hand every event-driven conversation an empty allowlist. The manager SHALL NOT create or require any RBAC verbs on roles or rolebindings.

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
- **THEN** only a namespaced Role/RoleBinding renders and the adapter can watch events only in the release namespace
