# mcp-toolset-model — delta

## MODIFIED Requirements

### Requirement: Wiring-level tool access resolves from the bound refs
A conversation's tool access SHALL come from its materialized bindings — the profile contributes nothing, having no capability fields. **Allowlist**: the concatenation of the `toolsets` refs' `tools` in ref order, deduped, first occurrence keeping its position. This is the WIRING'S CONTRIBUTION, not the final allowlist: the `toolsets` binding also carries a `mode` (`merge` | `overwrite`, default `merge`) saying how it composes with what the agent's own definition declares, and that composition happens in the runtime, which is the only component holding the repository. **MCP servers**: the `mcpConfigs` refs' servers merged in ref order, per server key, later ref winning on collision. A conversation with no bindings contributes no tools and no MCP servers. A bound `MCPConfig` in raw form (`configMapRef`/`secretRef`) SHALL be exclusive — combining it with any other config SHALL surface a condition naming the conflict, since a hand-written `mcp.json` cannot be composed.

#### Scenario: Bound toolsets are the wiring's contribution
- **WHEN** a pipeline binds two toolsets to a conversation
- **THEN** the work unit carries exactly those toolsets' tools, deduped in ref order, together with the mode that composes them with the agent's own

#### Scenario: The mode travels with the work unit
- **WHEN** a pipeline binds `toolsets` in `overwrite` mode
- **THEN** the work unit records that mode, so the runtime replaces the agent's declared tools rather than extending them

#### Scenario: Bound configs are the whole MCP
- **WHEN** a pipeline binds two MCPConfigs whose servers share a key
- **THEN** the compiled `mcp.json` contains the union, the later ref winning the shared key

#### Scenario: No binding means no contribution
- **WHEN** a conversation carries no `toolsets` and no `mcpConfigs` binding
- **THEN** the wiring contributes no tools and the runtime gets an empty `mcp.json`

#### Scenario: Raw configs refuse to combine
- **WHEN** a binding names a raw-form MCPConfig alongside another config
- **THEN** the conversation surfaces a condition naming the conflict instead of mounting a partial result
