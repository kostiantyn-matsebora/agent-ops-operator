## MODIFIED Requirements

### Requirement: Bindings materialize on the Conversation with lazy content resolution
Conversations SHALL snapshot both tooling bindings (their ref lists) from the ORIGINATING PIPELINE into the Conversation spec at creation — materialized per-conversation state, following the profileRef/channelRefs pattern; no `pipelineRef` is introduced. That Pipeline is the only source: no profile default, no baseline, no inheritance. A conversation whose Pipeline declared a binding carries it; one whose Pipeline did not carries none. Toolset and MCPConfig CONTENT SHALL be re-resolved at each use (MCP compilation, work-unit dispatch), so content edits reach existing conversations while re-wiring affects only new ones. This SHALL hold identically for every origination path — a signal of any kind through a claimed source, and a chat command naming a Pipeline — because all of them read the same Pipeline for the same fields.

#### Scenario: Pipeline bindings follow the signal
- **WHEN** a signal routes through a pipeline with `toolsets: {refs: [vm-observability]}` and `mcpConfigs: {refs: [vm-logs]}`
- **THEN** the created conversation's spec records both ref sets

#### Scenario: Content edits heal running conversations
- **WHEN** a bound MCPConfig's server URL is corrected while conversations referencing it exist
- **THEN** subsequent MCP compilation for those conversations uses the corrected URL

#### Scenario: A posted task carries the claiming Pipeline's wiring
- **WHEN** a `kind: task` signal is posted to a source claimed by a Pipeline that binds toolsets and mcpConfigs
- **THEN** the created conversation carries that Pipeline's profile, channel set, and both tooling bindings

#### Scenario: A Pipeline declaring nothing yields no bindings
- **WHEN** a conversation originates from a Pipeline with neither binding declared
- **THEN** it carries no bindings and dispatches with an empty allowlist, with nothing supplying a default
