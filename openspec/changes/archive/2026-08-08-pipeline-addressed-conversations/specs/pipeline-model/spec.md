# pipeline-model — delta

## MODIFIED Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline SHALL also be the SOLE source of its conversations' capabilities, via two optional stanzas of ordered refs: `spec.toolsets` (→ `MCPToolset` CRs, the allowlist) and `spec.mcpConfigs` (→ `MCPConfig` CRs, the MCP servers). Neither carries a mode and neither has a default — a Pipeline that declares no bindings gives its conversations no capabilities, and nothing supplies them elsewhere. A Pipeline SHALL be ADDRESSABLE by name: the task API creates a conversation against a named Pipeline, taking its profile, channel set, and capabilities from it. A Pipeline with neither sources nor channels carries no special meaning; it is simply a route nothing feeds. The Pipeline SHALL carry no credentials, no server or tool definitions, and no runtime selection (runtime stays `profile.runtimeRef → "default"`). A reconciler SHALL maintain a `Ready` condition (all references resolve, including toolset and mcpConfig refs) without creating any workload.

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

#### Scenario: A Pipeline is addressable by name
- **WHEN** the task API names a Ready Pipeline
- **THEN** the created conversation uses that Pipeline's profile, channel set, and capabilities

#### Scenario: A sourceless, channelless Pipeline is unremarkable
- **WHEN** a Pipeline names only a `profileRef` and capability stanzas
- **THEN** it is a route nothing feeds — addressable by the task API like any other, and carrying no per-profile default meaning

#### Scenario: Dangling tooling ref surfaces on Ready
- **WHEN** a Pipeline's `toolsets.refs` or `mcpConfigs.refs` names a CR that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

### Requirement: Pipeline-only resolution
Routing SHALL resolve wiring exclusively through Ready Pipelines: a source's signals route via the pipeline that claims it (unclaimed sources drop signals with a visible reason and a `Wired=False` condition); a channel's default profile for bare messages is its oldest Ready Pipeline's profile (channels in no pipeline answer bare messages with guidance). A `/<name> <task>` chat command SHALL address a PIPELINE by name — the Pipeline originates the conversation, so it supplies the profile AND the capabilities; addressing a profile would name something with no wiring and therefore nothing to grant. The agent listing SHALL enumerate Ready Pipelines rather than profiles, so it advertises only names a user can actually address. Thread replies continue to work regardless of wiring. No CR other than `Pipeline` carries standing wiring; `Conversation` fields are materialized per-conversation state, not wiring.

#### Scenario: Unclaimed source routes nothing
- **WHEN** a signal arrives for a source no Ready Pipeline references
- **THEN** no conversation is created, the response says so, and the source shows `Wired=False`

#### Scenario: A command addresses a pipeline and gets its capabilities
- **WHEN** a user sends `/some-pipeline do it` on a channel
- **THEN** the conversation uses that Pipeline's profile and carries its toolsets and mcpConfigs, rather than being created with none

#### Scenario: Commands work on unwired channels
- **WHEN** a user sends `/some-pipeline do it` on a channel referenced by no Pipeline
- **THEN** a conversation is created for that Pipeline, bound to the originating channel

#### Scenario: The listing advertises only addressable names
- **WHEN** a user asks for the agent listing
- **THEN** it names Ready Pipelines, not AgentProfiles — a profile name cannot be addressed

#### Scenario: Bare message on a pipeline channel
- **WHEN** a non-command message arrives on a channel referenced by a Ready Pipeline
- **THEN** the conversation uses the pipeline's profile and is bound to all the pipeline's channels
