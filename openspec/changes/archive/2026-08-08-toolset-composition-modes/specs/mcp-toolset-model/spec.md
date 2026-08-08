# mcp-toolset-model — delta

## REMOVED Requirements

### Requirement: Wiring-level tool access resolves from the bound refs
**Reason**: Its central claim — that the bound toolsets ARE the allowlist, and that a mode would name one behavior twice — is false for tools. The counterpart was never the profile; it is the agent definition's own `tools:` frontmatter, which a run also draws on. Verified against the CLI: `--allowedTools` is the sole permission authority and a definition's `tools:` neither widens nor narrows the session on its own, so the two halves compose only if something composes them. Replaced by the requirement below, whose scenarios say "contribution" where these said "whole".

**Migration**: None for existing objects — `toolsets.mode` defaults to `merge`, which reproduces the old behavior wherever an agent definition declares no `tools:`. Routes that must ignore a definition's declaration set `mode: overwrite`.

## ADDED Requirements

### Requirement: Wiring-level tool access is the bound refs' contribution
A conversation's wiring-level tool access SHALL come from its materialized bindings — the profile contributes nothing, having no capability fields. **Allowlist contribution**: the concatenation of the `toolsets` refs' `tools` in ref order, deduped, first occurrence keeping its position. This is the WIRING'S CONTRIBUTION, not the final allowlist: the `toolsets` binding also carries a `mode` (`merge` | `overwrite`, default `merge`) saying how it composes with what the agent's own definition declares, and that composition SHALL happen in the runtime, which is the only component holding the repository. A binding stored without a mode SHALL compose as `merge`. **MCP servers**: the `mcpConfigs` refs' servers merged in ref order, per server key, later ref winning on collision; that binding carries NO mode, because an agent definition declares no servers. A conversation with no bindings contributes no tools and no MCP servers. A bound `MCPConfig` in raw form (`configMapRef`/`secretRef`) SHALL be exclusive — combining it with any other config SHALL surface a condition naming the conflict, since a hand-written `mcp.json` cannot be composed.

#### Scenario: Bound toolsets are the wiring's contribution
- **WHEN** a pipeline binds two toolsets to a conversation
- **THEN** the work unit carries exactly those toolsets' tools, deduped in ref order, together with the mode that composes them with the agent's own

#### Scenario: The mode travels with the work unit
- **WHEN** a pipeline binds `toolsets` in `overwrite` mode
- **THEN** the work unit records that mode, so the runtime replaces the agent's declared tools rather than extending them

#### Scenario: An absent mode composes as merge
- **WHEN** a conversation's `toolsets` binding carries no mode
- **THEN** its work unit dispatches as `merge`, never as `overwrite` — an unset field must not strip what the agent declared

#### Scenario: Bound configs are the whole MCP
- **WHEN** a pipeline binds two MCPConfigs whose servers share a key
- **THEN** the compiled `mcp.json` contains the union, the later ref winning the shared key

#### Scenario: No binding means no contribution
- **WHEN** a conversation carries no `toolsets` and no `mcpConfigs` binding
- **THEN** the wiring contributes no tools and the runtime gets an empty `mcp.json`

#### Scenario: Raw configs refuse to combine
- **WHEN** a binding names a raw-form MCPConfig alongside another config
- **THEN** the conversation surfaces a condition naming the conflict instead of mounting a partial result
