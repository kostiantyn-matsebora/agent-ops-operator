# pipeline-model

## ADDED Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline SHALL carry no credentials, config, or runtime selection (runtime stays `profile.runtimeRef → "default"`). A reconciler SHALL maintain a `Ready` condition (all references resolve) without creating any workload.

#### Scenario: Signals fan out to every pipeline channel
- **WHEN** a Pipeline binds source `alertmanager` to channels `home-ops` and `web` and an alert fires
- **THEN** the resulting conversation carries channel bindings for both `home-ops` and `web` and uses the pipeline's profile

#### Scenario: Chat-originated conversations are pipeline-bound
- **WHEN** a user starts a conversation on a channel referenced by a Pipeline
- **THEN** the conversation is bound to all the Pipeline's channels, not just the originating one

#### Scenario: Dangling references surface on Ready
- **WHEN** a Pipeline references a SignalSource that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

### Requirement: Pipeline-first resolution with source-level fallback
Routing SHALL resolve the pipeline first: a source referenced by a Ready Pipeline uses the pipeline's channels and profile (the source's own `channelRef`/`profileRef` are inert while claimed); a source referenced by no Pipeline SHALL route exactly as before via its own refs. A channel referenced by no Pipeline SHALL originate single-channel conversations exactly as before.

#### Scenario: Unclaimed source keeps legacy routing
- **WHEN** a signal arrives for a source no Pipeline references
- **THEN** the conversation uses the source's own `channelRef` and `profileRef`, single-channel, unchanged

#### Scenario: Claimed source ignores its own refs
- **WHEN** a source with its own `channelRef` is referenced by a Ready Pipeline with different channels
- **THEN** conversations bind to the Pipeline's channels and the source's own refs have no effect

### Requirement: One pipeline per source
A SignalSource SHALL be claimed by at most one Pipeline: when a second Pipeline references an already-claimed source, the newer Pipeline SHALL report `SourceConflict=True` naming the contested source and the older claim SHALL keep routing. Channels MAY be referenced by multiple Pipelines; a conversation's binding set comes from the pipeline that originates it, and inbound on a multi-pipeline channel originates via the oldest Ready claimant (deterministic).

#### Scenario: Second claim on a source is refused
- **WHEN** two Pipelines reference the same SignalSource
- **THEN** the newer reports `SourceConflict=True` and the source's signals keep routing per the older Pipeline

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline
