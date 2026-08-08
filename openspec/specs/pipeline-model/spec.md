# pipeline-model

## Purpose

The Pipeline CRD: a credential-free wiring layer binding N signal sources and M channels to one profile — optionally also binding tool access (`toolsets`/`mcpConfigs` refs + mode) — with pipeline-only routing resolution, and at-most-one-pipeline-per-source claiming.

## Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline MAY additionally bind tool access via two symmetric optional stanzas, each `{mode: merge|overwrite (default merge), refs: [...]}`: `spec.toolsets` (refs to `MCPToolset` CRs, governing the allowlist) and `spec.mcpConfigs` (refs to `MCPConfig` CRs, extending or replacing the runtime's MCP servers for this wiring) — many-to-many in both directions. Tool-access BINDING (refs + mode) is wiring; all content stays in the referenced CRs. The Pipeline SHALL carry no credentials, no server or tool definitions, and no runtime selection (runtime stays `profile.runtimeRef → "default"`). A reconciler SHALL maintain a `Ready` condition (all references resolve, including toolset and mcpConfig refs) without creating any workload.

#### Scenario: Signals fan out to every pipeline channel
- **WHEN** a Pipeline binds source `alertmanager` to channels `home-ops` and `web` and an alert fires
- **THEN** the resulting conversation carries channel bindings for both `home-ops` and `web` and uses the pipeline's profile

#### Scenario: Chat-originated conversations are pipeline-bound
- **WHEN** a user starts a conversation on a channel referenced by a Pipeline
- **THEN** the conversation is bound to all the Pipeline's channels, not just the originating one

#### Scenario: Dangling references surface on Ready
- **WHEN** a Pipeline references a SignalSource that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

#### Scenario: Tooling binds per wiring, not per profile
- **WHEN** two Ready Pipelines route to the same profile, one with `toolsets`/`mcpConfigs` stanzas and one without
- **THEN** conversations from the first carry the bindings and conversations from the second use the profile's own tools and MCP

#### Scenario: Dangling tooling ref surfaces on Ready
- **WHEN** a Pipeline's `toolsets.refs` or `mcpConfigs.refs` names a CR that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

### Requirement: Pipeline-only resolution
Routing SHALL resolve wiring exclusively through Ready Pipelines: a source's signals route via the pipeline that claims it (unclaimed sources drop signals with a visible reason and a `Wired=False` condition); a channel's default profile for bare messages is its oldest Ready Pipeline's profile (channels in no pipeline answer bare messages with guidance; `/profile` commands and thread replies work regardless). No CR other than `Pipeline` carries standing wiring; `Conversation` fields are materialized per-conversation state, not wiring.

#### Scenario: Unclaimed source routes nothing
- **WHEN** a signal arrives for a source no Ready Pipeline references
- **THEN** no conversation is created, the response says so, and the source shows `Wired=False`

#### Scenario: Commands work on unwired channels
- **WHEN** a user sends `/some-profile do it` on a channel referenced by no Pipeline
- **THEN** a single-channel conversation is created for that profile exactly as before

#### Scenario: Bare message on a pipeline channel
- **WHEN** a non-command message arrives on a channel referenced by a Ready Pipeline
- **THEN** the conversation uses the pipeline's profile and is bound to all the pipeline's channels

### Requirement: One pipeline per source
A SignalSource SHALL be claimed by at most one Pipeline: when a second Pipeline references an already-claimed source, the newer Pipeline SHALL report `SourceConflict=True` naming the contested source and the older claim SHALL keep routing. Channels MAY be referenced by multiple Pipelines; a conversation's binding set comes from the pipeline that originates it, and inbound on a multi-pipeline channel originates via the oldest Ready claimant (deterministic).

#### Scenario: Second claim on a source is refused
- **WHEN** two Pipelines reference the same SignalSource
- **THEN** the newer reports `SourceConflict=True` and the source's signals keep routing per the older Pipeline

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline
