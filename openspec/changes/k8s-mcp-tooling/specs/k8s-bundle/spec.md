# k8s-bundle — delta

## MODIFIED Requirements

### Requirement: Each component is individually toggleable
Within an active bundle, `eventsAdapter.enabled`, `profile.enabled`, `rbac.enabled`, and `mcp.enabled` SHALL independently control their component's objects (all default `true`; the `mcpServers` sub-component that deploys the MCP server workload defaults to `false`). Cross-component references SHALL be values-resolvable so partial enablement works: the rendered Pipeline's `profileRef` defaults to the bundle's profile name but is overridable; RBAC binds the values-named runtime ServiceAccount; the MCP tooling stanzas are added to that Pipeline only when the MCP component is active; a render-time failure with a clear message SHALL occur when `eventsAdapter.source.create` is on but no profile name resolves.

#### Scenario: Events-only bundle
- **WHEN** the bundle is enabled with `profile.enabled=false`, `rbac.enabled=false`, and `eventsAdapter.source.profileRef` pointing at an operator-provided profile
- **THEN** only the SignalAdapter, its RBAC, the SignalSource, and the Pipeline claiming it render, wired to that profile

#### Scenario: Profile-only bundle
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the profile, runtime, SA, and RBAC render and the agent is usable via `/task` with no event ingestion

#### Scenario: MCP tooling without the events lane
- **WHEN** the bundle is enabled with `mcp.enabled=true` and `eventsAdapter.enabled=false`
- **THEN** the `MCPConfig` and `MCPToolset` render for operators to bind from their own Pipelines, and no Pipeline is created by the bundle

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render: the `k8s-engineer` AgentProfile (values-configurable name, `allowedTools` defaulting to `Read,Grep,Glob,Bash`, `maxTurns` 40, no repository, and **no `mcp` block — MCP reaches this profile through wiring, not through the profile**); a dedicated runtime ServiceAccount (default `agentops-runtime-k8s`); and, when `runtime.create` is on, an `AgentRuntime` (name defaulting to `default`, values-configured image and LLM credential Secret ref projected as env via `valueFrom` — the manager reads no Secrets) whose `serviceAccountName` is that SA. `runtime.create: false` SHALL support operators wiring the profile to an existing runtime via `runtimeRef` values.

#### Scenario: Profile executes under the bundle SA
- **WHEN** the bundle renders with defaults and a task addresses `k8s-engineer`
- **THEN** the conversation's runtime pod runs under the bundle's ServiceAccount, so the agent's in-cluster power is exactly what the bundle's RBAC component granted

#### Scenario: Bring-your-own runtime
- **WHEN** `profile.runtime.create=false` and `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the AgentProfile renders with that `runtimeRef` and no AgentRuntime or SA is created by the bundle

#### Scenario: The profile stays free of MCP config
- **WHEN** the bundle renders with the MCP component active
- **THEN** the `MCPConfig` is referenced by the rendered Pipeline's `mcpConfigs` binding and the AgentProfile itself declares no `mcp` block, so profiles stay reusable across differently-tooled routes
