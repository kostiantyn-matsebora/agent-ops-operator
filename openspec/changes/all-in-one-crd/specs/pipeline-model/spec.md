## MODIFIED Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline MAY additionally bind tool access via two symmetric optional stanzas, each `{mode: merge|overwrite (default merge), refs: [...]}`: `spec.toolsets` (refs to `MCPToolset` CRs, governing the allowlist) and `spec.mcpConfigs` (refs to `MCPConfig` CRs, extending or replacing the runtime's MCP servers for this wiring) — many-to-many in both directions. Tool-access BINDING (refs + mode) is wiring.

Each of the five referenced kinds MAY alternatively be declared INLINE on the Pipeline (`spec.channels[]`, `spec.signalSources[]`, `spec.profile`, `spec.toolsets.inline[]`, `spec.mcpConfigs.inline[]`), in which case the reconciler SHALL materialize the entry into a real CR of that kind owned by the Pipeline, and the materialized object SHALL join the pipeline's effective reference set before wiring resolution. Inline declarations are a SPELLING of the same model, not a second model: content is still resolved from real CRs at use time, so "refs snapshotted, content re-read" is unchanged. Exactly one of `profileRef` or `profile` SHALL be given; the other four MAY mix inline and reference forms freely.

The Pipeline SHALL carry no credentials and no runtime selection (runtime stays `profile.runtimeRef → "default"`): inline blocks carry secret REFERENCES only, exactly as the standalone CRs do, and the manager SHALL read no Secrets when reconciling them. A reconciler SHALL maintain a `Ready` condition (all references resolve, including toolset and mcpConfig refs, and no materialization conflict) and SHALL create no workload — the only objects it creates are the CRs materialized from inline declarations.

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

#### Scenario: Inline declarations resolve like references
- **WHEN** a Pipeline declares its channel and profile inline instead of by reference
- **THEN** the materialized `Channel` and `AgentProfile` are used for routing exactly as referenced ones would be, and the Pipeline reports `Ready=True`

#### Scenario: No workload and no credentials
- **WHEN** a Pipeline with inline blocks naming secret references is reconciled
- **THEN** no workload is created, no Secret is read, and the secret references are carried into the materialized CRs unchanged

### Requirement: One pipeline per source
A SignalSource SHALL be claimed by at most one Pipeline, whether that source is referenced by name or materialized from an inline declaration: when a second Pipeline references an already-claimed source, the newer Pipeline SHALL report `SourceConflict=True` naming the contested source and the older claim SHALL keep routing. Inline declaration SHALL NOT be a way to bypass the claim rule — a materialized source is claimed by its declaring Pipeline exactly as a referenced one is. Channels MAY be referenced by multiple Pipelines; a conversation's binding set comes from the pipeline that originates it, and inbound on a multi-pipeline channel originates via the oldest Ready claimant (deterministic).

#### Scenario: Second claim on a source is refused
- **WHEN** two Pipelines reference the same SignalSource
- **THEN** the newer reports `SourceConflict=True` and the source's signals keep routing per the older Pipeline

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline

#### Scenario: An inline source is claimed by its pipeline
- **WHEN** a Pipeline declares a signal source inline and a newer Pipeline references that materialized source by name
- **THEN** the newer Pipeline reports `SourceConflict=True` and the declaring Pipeline keeps routing
