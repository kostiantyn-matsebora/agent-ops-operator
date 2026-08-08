# vm-bundle — delta

## ADDED Requirements

### Requirement: The bundle ships a ready-made observability toolset
When any MCP component is active, the bundle SHALL render an `MCPToolset` (default name `vm-observability`, values-overridable) whose `tools` carry the tool namespaces of the enabled components only (`mcp__victorialogs__*` when vmlogs is on, `mcp__victoriametrics__*` when vmmetrics is on). Attaching the bundle's tools to a wiring SHALL thereby be a single Pipeline stanza — `mcpConfigs: {refs: [vm-logs, vm-metrics]}` plus `toolsets: {refs: [vm-observability]}` — with no profile edit required.

#### Scenario: Toolset tracks enabled components
- **WHEN** the bundle renders with `mcp.vmlogs.enabled=true` and `mcp.vmmetrics.enabled=false`
- **THEN** `vm-observability` grants only `mcp__victorialogs__*`

#### Scenario: One pipeline stanza attaches the bundle's tools
- **WHEN** an operator adds the `mcpConfigs` + `toolsets` stanzas to the Pipeline routing the bundle's SignalSource to their profile
- **THEN** that pipeline's conversations can query VictoriaLogs/VictoriaMetrics via MCP while the profile CR remains unedited

## MODIFIED Requirements

### Requirement: MCP components ship vmlogs and vmmetrics as referencable MCPConfig CRs
When active, the `mcp.vmlogs` and `mcp.vmmetrics` components SHALL each render an `MCPConfig` CR (default names `vm-logs` / `vm-metrics`, values-overridable) whose single server entry uses the FIXED server key `victorialogs` / `victoriametrics` respectively (the key is the `mcp__<key>__*` tool namespace in profile allowlists and SHALL NOT be values-configurable), `type: sse`, a values-configured URL, and values-passthrough `headers` supporting `valueFrom` secret references for authenticated endpoints (the manager reads no Secrets). An enabled MCP component with an empty URL SHALL fail rendering with a message naming the required value. The documented way to wire the bundle's tools into a wiring SHALL be the Pipeline tooling stanza (`mcpConfigs` refs + the bundle's `MCPToolset`); editing the profile directly (`mcp.configRefs` + allowlist entries) SHALL remain a documented alternative.

#### Scenario: MCPConfigs render for profiles to reference
- **WHEN** the bundle is enabled with `mcp.vmlogs.url` and `mcp.vmmetrics.url` set and an AgentProfile lists both CRs in `mcp.configRefs` with the matching `mcp__victorialogs__*`/`mcp__victoriametrics__*` allowlist entries
- **THEN** conversations under that profile get working victorialogs and victoriametrics MCP tools

#### Scenario: Missing endpoint fails loudly
- **WHEN** the bundle is enabled with `mcp.vmlogs.enabled=true` and `mcp.vmlogs.url` empty
- **THEN** `helm template`/`install` fails naming `vm-bundle.mcp.vmlogs.url` instead of rendering an MCPConfig that points nowhere

#### Scenario: Authenticated endpoint without manager Secret reads
- **WHEN** `mcp.vmmetrics.headers` carries an `Authorization` header with a `secretKeyRef`
- **THEN** the rendered MCPConfig embeds the `valueFrom` reference and the credential is resolved only in the runtime pod, never read by the manager

### Requirement: The bundle ships no profile — tool wiring targets the operator's own profile
The bundle SHALL NOT render an AgentProfile (user decision 2026-08-07, reverting an interim bundled-profile addition: the alert-handling profile is operator-owned and typically already exists). `defaultSource.profileRef` is strictly required when the default source is enabled. The documentation SHALL present the Pipeline tooling stanza (`mcpConfigs: {refs: [vm-logs, vm-metrics]}` + `toolsets: {refs: [vm-observability]}` on the operator's Pipeline) as the one manual wiring step, with direct profile editing (`mcp.configRefs` + the tool-namespace allowlist entries) as the documented alternative for setups not using wiring-level bindings.

#### Scenario: No profile objects from the bundle
- **WHEN** the bundle renders fully enabled
- **THEN** no `AgentProfile` object appears in the output

#### Scenario: Default source always requires an explicit profile
- **WHEN** `defaultSource.enabled=true` with an empty `profileRef`
- **THEN** rendering fails naming the value — there is no bundled profile to fall back to
