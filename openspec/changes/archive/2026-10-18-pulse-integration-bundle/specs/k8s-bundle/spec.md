## Purpose

Folded into this change alongside the Pulse bundle, for the same reason:
`pipelines.enabled` names one of the two CRD shapes a bundle's wiring can
take, even though the flag gates both — this rewrites it as `wiringEnabled`,
paired with `global.agentops.wiringMode`, across every bundle that ships one.

## MODIFIED Requirements

### Requirement: Each component is individually toggleable
Within an active bundle, `eventsAdapter.enabled`, `profile.enabled`, `mcp.enabled`, and `wiringEnabled` SHALL independently control their component's objects.

The first three SHALL default `true`. The `mcpServers` sub-component that deploys the MCP server workload SHALL default `true` alongside `mcp`: the two flip together so the config's URL always has an endpoint to default onto, which is the only reason the MCP component was previously off.

The endpoint guard SHALL remain and SHALL still fail loudly for `mcp.enabled` with no server workload and no `url`.

`wiringEnabled` SHALL default `false` — the exception among the four, because a route is the one component of this bundle that spends money and touches the cluster on its own. Demo mode SHALL force it on. Nothing else SHALL force it on.

The bundle SHALL expose no runtime-defaults or floor-account component. Those are the parent's `global.agentops.runtimeDefaults` and `runtimes:`.

It SHALL expose ONE stated setting for whether agents on its lane may CHANGE the cluster, which moves the server's read-only flag, that server's RBAC, the mutating toolset and which route ships. Each stays individually overridable, and none is derived from a release-wide value.

Cross-component references SHALL be values-resolvable so partial enablement works. Wiring the bundle renders SHALL reference only objects the bundle itself renders, plus values-supplied names omitted when unset.

A route that would span components the bundle cannot see SHALL be declared by the install, in the parent chart's `pipelines:`.

#### Scenario: Events-only bundle
- **WHEN** the bundle is enabled with `profile.enabled=false`, `mcp.enabled=false` and wiring off
- **THEN** only the SignalAdapter, its RBAC, and the SignalSource render, and the install claims that source from its own wiring

#### Scenario: Wiring without a profile
- **WHEN** wiring is enabled with `profile.enabled=false`
- **THEN** no Pipeline renders, because a Pipeline with no profile has no agent to run

#### Scenario: Profile-only bundle
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the profile renders and the agent is usable through any Pipeline naming it, executing on the parent's runtime

#### Scenario: A Kubernetes bundle can see Kubernetes by default
- **WHEN** the bundle is enabled with defaults
- **THEN** the `MCPConfig`, the read toolset, and the MCP server workload all render, so the install does not look complete while lacking the cluster access path the project prefers

#### Scenario: MCP tooling without the events lane
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the `MCPConfig` and toolsets render for operators to bind from their own Pipelines
