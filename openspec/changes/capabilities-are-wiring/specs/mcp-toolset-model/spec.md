# mcp-toolset-model — delta

## MODIFIED Requirements

### Requirement: Wiring-level tool access resolves per binding mode
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
