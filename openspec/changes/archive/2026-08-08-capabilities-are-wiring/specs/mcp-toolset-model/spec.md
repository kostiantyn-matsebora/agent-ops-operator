# mcp-toolset-model — delta

## RENAMED Requirements

- FROM: `### Requirement: Wiring-level tool access resolves per binding mode`
- TO: `### Requirement: Wiring-level tool access resolves from the bound refs`

## MODIFIED Requirements

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

### Requirement: Bindings materialize on the Conversation with lazy content resolution
Conversations originated by a Pipeline SHALL snapshot both tooling bindings (their ref lists) into the Conversation spec at creation — materialized per-conversation state, following the profileRef/channelRefs pattern; no `pipelineRef` is introduced. Toolset and MCPConfig CONTENT SHALL be re-resolved at each use (MCP compilation, work-unit dispatch), so content edits reach existing conversations while pipeline re-wiring affects only new ones. A conversation created through `POST /task` naming a pipeline SHALL carry that pipeline's tooling bindings alongside its channel set — having named the pipeline, the caller gets its wiring, not half of it. Conversations with NO routing pipeline (`POST /task` without one, `/<profile>` commands) SHALL resolve the named profile's capability-only Pipeline — its baseline — and snapshot those bindings; where no baseline exists they carry none and therefore have no capabilities, since the profile itself declares none.

#### Scenario: Pipeline bindings follow the signal
- **WHEN** a signal routes through a pipeline with `toolsets: {refs: [vm-observability]}` and `mcpConfigs: {refs: [vm-logs]}`
- **THEN** the created conversation's spec records both ref sets

#### Scenario: Content edits heal running conversations
- **WHEN** a bound MCPConfig's server URL is corrected while conversations referencing it exist
- **THEN** subsequent MCP compilation for those conversations uses the corrected URL

#### Scenario: Task API with a pipeline carries its whole wiring
- **WHEN** `POST /task` names a pipeline that binds toolsets and mcpConfigs
- **THEN** the created conversation carries that pipeline's channel set AND both tooling bindings

#### Scenario: Task API without a pipeline resolves the baseline
- **WHEN** a conversation is created via `POST /task` with no pipeline named, against a profile that has a capability-only Pipeline
- **THEN** it carries that baseline's bindings, so the agent is equipped without any routing wiring
