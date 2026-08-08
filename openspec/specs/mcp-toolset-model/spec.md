# mcp-toolset-model

## Purpose

Wiring-level tool access: the `MCPToolset` CRD (a reusable, server-definition-free list of tool patterns), the two mode-less Pipeline bindings (`toolsets` for the allowlist, `mcpConfigs` for MCP servers), their materialization onto the Conversation with lazy content resolution, and how compilation and dispatch apply the effective tooling.

## Requirements

### Requirement: MCPToolset CRD declares a reusable tool list
An `MCPToolset` CRD SHALL declare a named LIST of tool patterns (`spec.tools`): MCP namespaces like `mcp__victorialogs__*` and/or built-in tool names like `Bash`. It SHALL carry NO MCP server definitions — server definitions belong exclusively to `MCPConfig` CRs. The patterns are opaque to the manager (passed through to the runtime exactly like `allowedTools` today), so the CRD needs no resolution status.

#### Scenario: A bundle-shipped toolset is a single CR
- **WHEN** an `MCPToolset` with `tools: [mcp__victorialogs__*, mcp__victoriametrics__*]` is applied
- **THEN** it is referencable from any number of Pipelines' `toolsets` bindings

#### Scenario: Toolsets may grant built-in tools
- **WHEN** an `MCPToolset` lists `tools: [Read, Grep, mcp__victorialogs__*]`
- **THEN** all three entries participate in allowlist resolution — the toolset is not restricted to MCP namespaces

### Requirement: Wiring-level tool access resolves from the bound refs
A conversation's tool access SHALL come from its materialized bindings ALONE — the profile contributes nothing, having no capability fields. **Allowlist**: the concatenation of the `toolsets` refs' `tools` in ref order, deduped, first occurrence keeping its position. **MCP servers**: the `mcpConfigs` refs' servers merged in ref order, per server key, later ref winning on collision. Neither binding carries a mode: with one source of capabilities there is nothing to compose against, so `merge` and `overwrite` would name the same behavior. A conversation with no bindings has no tools and no MCP servers. A bound `MCPConfig` in raw form (`configMapRef`/`secretRef`) SHALL be exclusive — combining it with any other config SHALL surface a condition naming the conflict, since a hand-written `mcp.json` cannot be composed.

#### Scenario: Bound toolsets are the whole allowlist
- **WHEN** a pipeline binds two toolsets to a conversation
- **THEN** the work unit's `allowedTools` is exactly those toolsets' tools, deduped in ref order

#### Scenario: Bound configs are the whole MCP
- **WHEN** a pipeline binds two MCPConfigs whose servers share a key
- **THEN** the compiled `mcp.json` contains the union, the later ref winning the shared key

#### Scenario: No binding means no capability
- **WHEN** a conversation carries no `toolsets` and no `mcpConfigs` binding
- **THEN** its work unit carries an empty allowlist and its runtime gets an empty `mcp.json`

#### Scenario: Raw configs refuse to combine
- **WHEN** a binding names a raw-form MCPConfig alongside another config
- **THEN** the conversation surfaces a condition naming the conflict instead of mounting a partial result

### Requirement: Bindings materialize on the Conversation with lazy content resolution
Conversations SHALL snapshot both tooling bindings (their ref lists) from the ORIGINATING PIPELINE into the Conversation spec at creation — materialized per-conversation state, following the profileRef/channelRefs pattern; no `pipelineRef` is introduced. That Pipeline is the only source: no profile default, no baseline, no inheritance. A conversation whose Pipeline declared a binding carries it; one whose Pipeline did not carries none. Toolset and MCPConfig CONTENT SHALL be re-resolved at each use (MCP compilation, work-unit dispatch), so content edits reach existing conversations while re-wiring affects only new ones.

#### Scenario: Pipeline bindings follow the signal
- **WHEN** a signal routes through a pipeline with `toolsets: {refs: [vm-observability]}` and `mcpConfigs: {refs: [vm-logs]}`
- **THEN** the created conversation's spec records both ref sets

#### Scenario: Content edits heal running conversations
- **WHEN** a bound MCPConfig's server URL is corrected while conversations referencing it exist
- **THEN** subsequent MCP compilation for those conversations uses the corrected URL

#### Scenario: The task API addresses a Pipeline and carries its wiring
- **WHEN** `POST /task` names a Pipeline that binds toolsets and mcpConfigs
- **THEN** the created conversation carries that Pipeline's profile, channel set, and both tooling bindings

#### Scenario: A Pipeline declaring nothing yields no bindings
- **WHEN** a conversation originates from a Pipeline with neither binding declared
- **THEN** it carries no bindings and dispatches with an empty allowlist, with nothing supplying a default

### Requirement: Compilation and dispatch apply the effective tooling
Every conversation with an `mcpConfigs` binding SHALL compile into a conversation-owned ConfigMap `agentops-mcp-conv-<conversation>` (ownerRef → Conversation, GC'd with it); the profile-keyed `agentops-mcp-<profile>` ConfigMap SHALL NOT be created, since profiles declare no MCP. Secret-backed header values SHALL still compile to `valueFrom` env placeholders — the manager reads no Secrets. Work-unit dispatch SHALL compute the allowlist from the bound toolsets at dispatch time, so toolset edits apply from the next work unit with no pod restart. A binding ref that fails to resolve SHALL fail visibly (conversation condition), never degrade silently to reduced tooling.

#### Scenario: Every MCP-bound conversation owns its ConfigMap
- **WHEN** two pipelines with different `mcpConfigs` bindings route to one profile and each creates a conversation
- **THEN** each mounts its own `agentops-mcp-conv-<name>` and no profile-keyed ConfigMap exists

#### Scenario: Toolset content edits reach running conversations
- **WHEN** a bound MCPToolset gains a tool while conversations referencing it exist
- **THEN** their next work unit carries it, with no runtime pod restart

#### Scenario: Missing binding ref fails the work visibly
- **WHEN** a conversation's bound MCPToolset or MCPConfig is deleted and a new work unit is dispatched
- **THEN** the failure surfaces on the conversation rather than proceeding with silently reduced tooling
