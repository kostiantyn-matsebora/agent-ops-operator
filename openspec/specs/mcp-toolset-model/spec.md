# mcp-toolset-model

## Purpose

Wiring-level tool access: the `MCPToolset` CRD (a reusable, server-definition-free list of tool patterns), the two independently moded Pipeline bindings (`toolsets` for the allowlist, `mcpConfigs` for MCP servers), their materialization onto the Conversation with lazy content resolution, and how compilation and dispatch apply the effective tooling.

## Requirements

### Requirement: MCPToolset CRD declares a reusable tool list
An `MCPToolset` CRD SHALL declare a named LIST of tool patterns (`spec.tools`): MCP namespaces like `mcp__victorialogs__*` and/or built-in tool names like `Bash`. It SHALL carry NO MCP server definitions — server definitions belong exclusively to `MCPConfig` CRs. The patterns are opaque to the manager (passed through to the runtime exactly like `allowedTools` today), so the CRD needs no resolution status.

#### Scenario: A bundle-shipped toolset is a single CR
- **WHEN** an `MCPToolset` with `tools: [mcp__victorialogs__*, mcp__victoriametrics__*]` is applied
- **THEN** it is referencable from any number of Pipelines' `toolsets` bindings

#### Scenario: Toolsets may grant built-in tools
- **WHEN** an `MCPToolset` lists `tools: [Read, Grep, mcp__victorialogs__*]`
- **THEN** all three entries participate in allowlist resolution — the toolset is not restricted to MCP namespaces

### Requirement: Wiring-level tool access resolves per binding mode
A conversation's tool access SHALL resolve from the profile plus its two materialized bindings, each independently moded. **Allowlist** (from `toolsets` refs `T1..Tn` in order): `merge` = the profile's `allowedTools` entries (comma-split) unioned with each toolset's `tools` (dedup, first occurrence keeps position); `overwrite` = the toolsets' tools alone, ignoring the profile's allowlist including built-ins. **MCP servers** (from `mcpConfigs` refs `C1..Cn` in order): `merge` = the profile's compiled MCP map overlaid by each config's servers (per-server-key, later wins — bound configs override the profile on collision); `overwrite` = the bound configs alone, ignoring the profile's `mcp` entirely. A profile whose `mcp` uses raw `configMapRef`/`secretRef` SHALL be an error under `mcpConfigs` `merge` (surfaced as a visible conversation condition naming the incompatibility, never a silent partial merge); `overwrite` SHALL work over such profiles. The two bindings are independent — either may be present, absent, or differently moded.

#### Scenario: Merge adds wiring tools to the profile
- **WHEN** a profile with `allowedTools: "Read,Bash"` and one MCP server runs under a pipeline binding `mcpConfigs: {refs: [vm-logs]}` and `toolsets: {refs: [vm-observability]}` in merge mode
- **THEN** conversations get the profile's server plus `vm-logs`' servers, and the allowlist contains `Read`, `Bash`, and the toolset's entries exactly once each

#### Scenario: Overwrite replaces the profile's tools
- **WHEN** the same profile runs under a pipeline whose two bindings use `overwrite` mode
- **THEN** conversations get only the bound configs' servers and the toolsets' allowlist — the profile's own `Read,Bash` and server are absent

#### Scenario: Independent modes compose
- **WHEN** a pipeline binds `toolsets` in `merge` mode and `mcpConfigs` in `overwrite` mode
- **THEN** the allowlist extends the profile's while the MCP servers come from the bound configs alone

#### Scenario: Raw-form profile refuses to merge servers
- **WHEN** a `merge`-mode `mcpConfigs` binding routes to a profile whose MCP is a raw `configMapRef`
- **THEN** the conversation surfaces a condition naming the incompatibility instead of mounting a half-merged config

### Requirement: Bindings materialize on the Conversation with lazy content resolution
Conversations originated by a Pipeline SHALL snapshot both tooling bindings (mode + refs each) into the Conversation spec at creation — materialized per-conversation state, following the profileRef/channelRefs pattern; no `pipelineRef` is introduced. Toolset and MCPConfig CONTENT SHALL be re-resolved at each use (MCP compilation, work-unit dispatch), so content edits reach existing conversations while pipeline re-wiring affects only new ones. Conversations with no originating pipeline (`POST /task`, `/profile` commands on unwired channels) SHALL carry no bindings and use the profile's own tools and MCP unchanged.

#### Scenario: Pipeline bindings follow the signal
- **WHEN** a signal routes through a pipeline with `toolsets: {mode: merge, refs: [vm-observability]}` and `mcpConfigs: {refs: [vm-logs]}`
- **THEN** the created conversation's spec records both bindings' modes and ref sets

#### Scenario: Content edits heal running conversations
- **WHEN** a bound MCPConfig's server URL is corrected while conversations referencing it exist
- **THEN** subsequent MCP compilation for those conversations uses the corrected URL

#### Scenario: Task-API conversations are unaffected
- **WHEN** a conversation is created via `POST /task`
- **THEN** it has no bindings and behaves exactly as before this change

### Requirement: Compilation and dispatch apply the effective tooling
Conversations WITHOUT an `mcpConfigs` binding SHALL keep the existing shared, profile-owned ConfigMap `agentops-mcp-<profile>` byte-identical (existing tests pin it) — a toolsets-only binding changes nothing MCP-side. Conversations WITH an `mcpConfigs` binding SHALL compile their effective MCP into a conversation-owned ConfigMap `agentops-mcp-conv-<conversation>` (ownerRef → Conversation, GC'd with it) mounted in place of the profile CM, with secret-backed header values still compiling to `valueFrom` env placeholders (the manager reads no Secrets). Work-unit dispatch SHALL compute the effective `allowedTools` per the `toolsets` binding at dispatch time (allowlist changes need no pod restart). A binding ref that fails to resolve at use time SHALL fail visibly (conversation condition), never degrade silently to profile-only tooling.

#### Scenario: Binding-less path is byte-identical
- **WHEN** the envtest suite renders MCP ConfigMaps and work units for conversations without bindings after this change
- **THEN** names, owners, content, and `WorkUnit.allowedTools` are unchanged

#### Scenario: Config-bound conversations get their own ConfigMap
- **WHEN** two pipelines with different `mcpConfigs` bindings route to the same profile and each creates a conversation
- **THEN** each conversation mounts its own `agentops-mcp-conv-<name>` and neither touches `agentops-mcp-<profile>`

#### Scenario: Missing binding ref fails the work visibly
- **WHEN** a conversation's bound MCPToolset or MCPConfig is deleted and a new work unit is dispatched
- **THEN** the failure surfaces on the conversation rather than proceeding with silently reduced tooling
