# pipeline-model — delta

## MODIFIED Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline SHALL also be the SOLE source of its conversations' capabilities, via two optional stanzas of ordered refs: `spec.toolsets` (→ `MCPToolset` CRs, the allowlist) and `spec.mcpConfigs` (→ `MCPConfig` CRs, the MCP servers). Neither carries a mode — profiles declare no capabilities, so there is nothing to compose against. A Pipeline with NEITHER sources NOR channels is legal and meaningful: it declares its profile's baseline capabilities for conversations that have no routing pipeline. The Pipeline SHALL carry no credentials, no server or tool definitions, and no runtime selection (runtime stays `profile.runtimeRef → "default"`). A reconciler SHALL maintain a `Ready` condition (all references resolve, including toolset and mcpConfig refs) without creating any workload.

#### Scenario: Signals fan out to every pipeline channel
- **WHEN** a Pipeline binds source `alertmanager` to channels `home-ops` and `web` and an alert fires
- **THEN** the resulting conversation carries channel bindings for both `home-ops` and `web` and uses the pipeline's profile

#### Scenario: Chat-originated conversations are pipeline-bound
- **WHEN** a user starts a conversation on a channel referenced by a Pipeline
- **THEN** the conversation is bound to all the Pipeline's channels, not just the originating one

#### Scenario: Dangling references surface on Ready
- **WHEN** a Pipeline references a SignalSource that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

#### Scenario: Capabilities bind per route
- **WHEN** two Ready Pipelines route to the same profile with different `toolsets`
- **THEN** conversations from each carry exactly that Pipeline's tools, and the profile declares none

#### Scenario: A sourceless, channelless Pipeline is a capability declaration
- **WHEN** a Pipeline names only a `profileRef` and capability stanzas
- **THEN** it is Ready and supplies that profile's baseline capabilities to conversations with no routing pipeline

#### Scenario: Dangling tooling ref surfaces on Ready
- **WHEN** a Pipeline's `toolsets.refs` or `mcpConfigs.refs` names a CR that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference
