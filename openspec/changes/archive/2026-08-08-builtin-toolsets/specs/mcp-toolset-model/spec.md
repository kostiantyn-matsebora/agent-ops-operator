# mcp-toolset-model — delta

## MODIFIED Requirements

### Requirement: Bindings materialize on the Conversation with lazy content resolution
Conversations originated by a Pipeline SHALL snapshot both tooling bindings (mode + refs each) into the Conversation spec at creation — materialized per-conversation state, following the profileRef/channelRefs pattern; no `pipelineRef` is introduced. Toolset and MCPConfig CONTENT SHALL be re-resolved at each use (MCP compilation, work-unit dispatch), so content edits reach existing conversations while pipeline re-wiring affects only new ones. A conversation created through `POST /task` naming a pipeline SHALL carry that pipeline's tooling bindings alongside its channel set — having named the pipeline, the caller gets its wiring, not half of it. Conversations with no originating pipeline (`POST /task` without one, `/profile` commands on unwired channels) SHALL carry no bindings and use the profile's own tooling unchanged.

#### Scenario: Pipeline bindings follow the signal
- **WHEN** a signal routes through a pipeline with `toolsets: {mode: merge, refs: [vm-observability]}` and `mcpConfigs: {refs: [vm-logs]}`
- **THEN** the created conversation's spec records both bindings' modes and ref sets

#### Scenario: Content edits heal running conversations
- **WHEN** a bound MCPConfig's server URL is corrected while conversations referencing it exist
- **THEN** subsequent MCP compilation for those conversations uses the corrected URL

#### Scenario: Task API with a pipeline carries its whole wiring
- **WHEN** `POST /task` names a pipeline that binds toolsets and mcpConfigs
- **THEN** the created conversation carries that pipeline's channel set AND both tooling bindings

#### Scenario: Task API without a pipeline stays binding-less
- **WHEN** a conversation is created via `POST /task` with no pipeline named
- **THEN** it carries no bindings and behaves exactly as before this change
