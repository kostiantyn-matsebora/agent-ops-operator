# k8s-bundle — delta

## MODIFIED Requirements

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
