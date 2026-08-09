# k8s-bundle — delta

## MODIFIED Requirements

### Requirement: The k8s bundle ships as a self-gated subchart, off by default and on in demo mode
A Helm subchart at `chart/charts/k8s-bundle/` SHALL package the Kubernetes agent experience as three components — events signal source, k8s-engineer profile, and its MCP tooling. Every bundle template SHALL gate on `enabled OR global.demo.enabled` (self-gating, not a Helm `condition:`), with `k8s-bundle.enabled` defaulting to `false`. The parent chart's demo toggle SHALL live at `global.demo.enabled` and there SHALL be no `chart/templates/demo.yaml` — demo mode means exactly "the bundle with its defaults", which includes read-only RBAC resolved by the parent. Explicit `k8s-bundle.*` values SHALL still apply when enabled via demo.

The bundle SHALL NOT render the agent's execution substrate. The `AgentRuntime`, the runtime ServiceAccount, the LLM credential Secret, and the runtime's RBAC are the parent chart's (`agent-runtime-ownership`), so a default install renders those and still renders nothing from this bundle.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, AgentProfile, Pipeline, MCP object, or RBAC object from the bundle is rendered — while the parent's `AgentRuntime` and runtime ServiceAccount do render, because they are not the bundle's

#### Scenario: Demo mode enables the bundle read-only
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** the bundle's components render, the parent's `AgentRuntime` named `default` carries the configured LLM credential env, the runtime SA holds read-only bindings, and the bundle's addressable Pipeline is askable via `POST /task` — a task names that Pipeline, never the profile

#### Scenario: Bundle without demo
- **WHEN** the chart is installed with `k8s-bundle.enabled=true` and `global.demo.enabled=false`
- **THEN** the same bundle components render — demo mode is an enablement path, not a distinct feature set — and the runtime SA holds no bindings unless `global.agentops.runtime.rbacMode` is set

### Requirement: Each component is individually toggleable
Within an active bundle, `eventsAdapter.enabled`, `profile.enabled`, and `mcp.enabled` SHALL independently control their component's objects. All three SHALL default `true`, and the `mcpServers` sub-component that deploys the MCP server workload SHALL default `true` alongside `mcp`: the two flip together so the config's URL always has an endpoint to default onto, which is the only reason the MCP component was previously off. The endpoint guard SHALL remain and SHALL still fail loudly for `mcp.enabled` with no server workload and no `url`.

The bundle SHALL expose no runtime or runtime-RBAC component; those toggles are the parent's `runtime.enabled` and `global.agentops.runtime.rbacMode`.

Cross-component references SHALL be values-resolvable so partial enablement works. The bundle SHALL render no `Pipeline` of its own: wiring names sources and channels that come from other bundles, so it is declared by the install, not by any component of it.

#### Scenario: Events-only bundle
- **WHEN** the bundle is enabled with `profile.enabled=false` and `mcp.enabled=false`
- **THEN** only the SignalAdapter, its RBAC, and the SignalSource render, and the install claims that source from its own wiring

#### Scenario: Profile-only bundle
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the profile renders and the agent is usable through any Pipeline naming it, executing on the parent's runtime

#### Scenario: A Kubernetes bundle can see Kubernetes by default
- **WHEN** the bundle is enabled with defaults
- **THEN** the `MCPConfig`, the read toolset, and the MCP server workload all render, so the install does not look complete while lacking the cluster access path the project prefers

#### Scenario: MCP tooling without the events lane
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the `MCPConfig` and toolsets render for operators to bind from their own Pipelines

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render exactly one object: the `k8s-engineer` `AgentProfile` (values-configurable name, `maxTurns`, no repository, and **no capabilities** — no `allowedTools`, no `mcp`). It SHALL render no `AgentRuntime`, no ServiceAccount, and no credential Secret; the profile executes on the parent chart's runtime.

Because the profile has NO repository, no agent definition file can be resolved for it. The component SHALL therefore support an inline role (`systemPrompt`) and ship a sensible default, so the shipped agent is not personality-free: a conversation woken by a cluster event would otherwise arrive with no instructions at all.

`profile.runtimeRef` SHALL remain, naming a runtime other than the parent's `default` — a higher-trust or different-vendor runtime the operator applied. Left empty, the profile emits no `runtimeRef` and falls back to `default`, which the parent guarantees exists whenever `runtime.enabled` is true.

#### Scenario: Profile executes under the release's runtime SA
- **WHEN** the bundle renders with defaults and a task reaches `k8s-engineer`
- **THEN** the conversation's runtime pod runs under `global.agentops.runtime.serviceAccountName`, so the agent's in-cluster power is exactly what `global.agentops.runtime.rbacMode` granted

#### Scenario: The profile component renders one object
- **WHEN** the bundle renders with `profile.enabled=true`
- **THEN** the component's output is the `AgentProfile` alone, and no `AgentRuntime`, ServiceAccount, or Secret carries a bundle label

#### Scenario: Pointing at a different runtime
- **WHEN** `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the `AgentProfile` renders with that `runtimeRef` and the parent's `default` runtime is left unused by this profile

#### Scenario: Fallback needs no wiring
- **WHEN** `profile.runtimeRef` is empty and `runtime.enabled` is true
- **THEN** the profile emits no `runtimeRef` and resolves the parent's `default` runtime

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

### Requirement: Demo values migrate to bundle paths
The pre-bundle demo values SHALL move: `demo.enabled` → `global.demo.enabled`, `demo.readOnlyRbac` → `global.agentops.runtime.rbacMode` (true ≙ `readonly`). The runtime-shaped demo values SHALL land in the parent's `runtime:` block rather than in this bundle: `demo.runtimeImage` → `runtime.image`, `demo.credentialsSecret.*` → `runtime.credentialsSecret.*`, inherited `persistence` → automatic, inherited `runtimeIdleTtlMinutes` → the manager default with `runtime.idleTtlMinutes` as an override.

The chart major version SHALL be bumped and the README SHALL carry a migration table covering BOTH hops, so an operator who moved a value into `k8s-bundle.profile.runtime.*` in 2.x can find where it went. That table SHALL lead with the two upgrade-visible effects that are not value renames: the runtime ServiceAccount changes name (bundle-named bindings are replaced by global-named ones), and an install that enabled the bundle without configuring MCP gains an MCP server workload.

Upgrading SHALL preserve semantics: the `AgentRuntime` named `default` re-renders equivalently from the parent, so existing conversations keep resolving their runtime.

#### Scenario: Upgraded demo release keeps working
- **WHEN** a release running chart 3.x with the bundle enabled upgrades and adopts the new values paths
- **THEN** the agent flow works unchanged, the `default` runtime re-renders from the parent, and the bundle-named ServiceAccount and its bindings are removed by the upgrade

#### Scenario: Both migration hops are findable
- **WHEN** an operator looks up a value they set at `k8s-bundle.profile.runtime.*`
- **THEN** the migration table names its 4.0 location, rather than only documenting the 1.x → 2.x hop

## REMOVED Requirements

### Requirement: RBAC is read-only by default with an explicit full mode
**Reason**: The runtime ServiceAccount's RBAC is not a Kubernetes-bundle concern — it is the power of the one identity every agent in the release executes as, whichever bundle originated the conversation. Keeping `rbac.mode` here forced the bundle to also own the SA it binds, which is what made a second runtime identity exist.

**Migration**: `k8s-bundle.rbac.mode` → `global.agentops.runtime.rbacMode`; `k8s-bundle.rbac.enabled: false` → `rbacMode: none`. The modes keep their exact meanings and rules, `full` is still never a default, and the requirement is restated in `agent-runtime-ownership` against the parent's SA. `eventsAdapter.rbac` is a different block and is unaffected.
