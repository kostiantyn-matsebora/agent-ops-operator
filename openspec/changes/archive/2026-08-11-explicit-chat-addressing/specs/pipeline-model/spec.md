## MODIFIED Requirements

### Requirement: Pipeline-only resolution
Routing SHALL resolve wiring exclusively through Ready Pipelines: a source's signals route via EVERY Ready Pipeline that lists it, and a source no Ready Pipeline lists drops its signals with a visible reason and a `Wired=False` condition. Resolution SHALL NOT fall back to pipeline creation order in any lane: the "oldest claimant" tiebreak is REMOVED, because a source no longer has a single claimant to pick.

This applies uniformly to every source kind, INCLUDING chat sources — a conversation originated by a general-surface chat message resolves through the Pipelines listing that chat `SignalSource`, exactly as an alert resolves through the Pipelines listing its alert source. Chat differs in ONE respect, and only for a message addressing no Pipeline: it is routed only when exactly one Ready Pipeline serves the source, because a person is owed a single answer and can name the agent they want.

A `Channel` SHALL NOT supply a default profile, and inbound resolution SHALL NOT fall back to pipeline creation order: the previous "oldest Ready Pipeline referencing this channel" tiebreak is REMOVED, because origination no longer happens on a channel. Channels remain shareable across Pipelines for delivery and mirroring, unaffected. Thread replies continue to resolve through the conversation's own thread binding, independent of any pipeline lookup.

A `/<name> <task>` chat command SHALL address a PIPELINE by name — the Pipeline originates the conversation, so it supplies the profile AND the capabilities; addressing a profile would name something with no wiring and therefore nothing to grant. The agent listing SHALL enumerate Ready Pipelines rather than profiles, so it advertises only names a user can actually address. No CR other than `Pipeline` carries standing wiring; `Conversation` fields are materialized per-conversation state, not wiring.

#### Scenario: Unclaimed source routes nothing
- **WHEN** a signal arrives for a source no Ready Pipeline references
- **THEN** no conversation is created, the response says so, and the source shows `Wired=False`

#### Scenario: Unclaimed chat source routes nothing
- **WHEN** a general-surface chat message arrives for a chat source no Ready Pipeline lists
- **THEN** no conversation is created, the source shows `Wired=False`, and the drop reason reaches the originating surface

#### Scenario: A command addresses a pipeline and gets its capabilities
- **WHEN** a user sends `/some-pipeline do it` on a channel
- **THEN** the conversation uses that Pipeline's profile and carries its toolsets and mcpConfigs, rather than being created with none

#### Scenario: Commands work through the chat source
- **WHEN** a user sends `/some-pipeline do it` on a channel whose chat source is listed by a Ready Pipeline
- **THEN** a conversation is created for that Pipeline through the signal path, bound to the originating channel

#### Scenario: The listing advertises only addressable names
- **WHEN** a user asks for the agent listing
- **THEN** it names Ready Pipelines, not AgentProfiles — a profile name cannot be addressed

#### Scenario: Bare message resolves only when one pipeline serves the surface
- **WHEN** a non-command message arrives on a channel's general surface and exactly one Ready Pipeline lists the chat source
- **THEN** the conversation uses that Pipeline's profile and is bound to all its channels

#### Scenario: No creation-order tiebreak remains anywhere
- **WHEN** two Ready Pipelines list the same source
- **THEN** neither is preferred by creation timestamp in any lane

#### Scenario: Shared channels need no tiebreak
- **WHEN** two Ready Pipelines both reference channel `web` for delivery
- **THEN** neither ordering nor creation timestamps affect inbound resolution, because neither claims origination on it

#### Scenario: Replies bypass pipeline resolution entirely
- **WHEN** a user replies inside an existing thread
- **THEN** the input is appended to that thread's conversation with no pipeline lookup

## REMOVED Requirements

### Requirement: One pipeline per source
**Reason**: Exclusivity existed to keep a single invisible default for bare chat messages, and it charged every source kind for it. Whether two Pipelines watch one source is the adopter's decision; the ambiguity it guarded against is now handled where it actually occurs, in the chat lane, by refusing rather than guessing.

**Migration**: A Pipeline previously reporting `SourceConflict=True` becomes `Ready=True` and its sources begin routing to it. An install that relied on the younger Pipeline being inert MUST drop the contested source from every Pipeline but the intended one.

## ADDED Requirements

### Requirement: Sources are shareable and signals fan out
A `SignalSource` MAY be listed by any number of Ready Pipelines, of any signal kind. Doing so SHALL NOT produce a conflict condition and SHALL NOT affect any Pipeline's `Ready`. Listing a source means "I watch this" — it makes the source wired and, on a chat surface, makes the Pipeline addressable there — not "I own this".

A signal admitted on a source served by several Ready Pipelines SHALL produce one conversation PER Pipeline, each carrying that Pipeline's own profile, channel set and capabilities. Per-source ingest policy — fingerprint cooldown and signature grouping — SHALL be evaluated ONCE, before the fan-out, so a fingerprint is admitted once and then delivered to each server rather than being suppressed for all but the first.

Channels MAY likewise be referenced by multiple Pipelines; a conversation's binding set comes from the Pipeline that originated it.

#### Scenario: Two pipelines watching one alert source both investigate
- **WHEN** an alert arrives on a source two Ready Pipelines list
- **THEN** two conversations are created, one per Pipeline, each with its own profile and capabilities, and neither Pipeline reports a conflict

#### Scenario: Cooldown is not spent by the first server
- **WHEN** a fingerprint is admitted on a source two Ready Pipelines list
- **THEN** both Pipelines receive it, and the fingerprint is recorded as admitted once for the source

#### Scenario: Several pipelines serve one chat surface
- **WHEN** two Ready Pipelines both list the same chat SignalSource
- **THEN** neither reports a conflict, both stay `Ready=True`, and both appear in the surface's listing of available agents

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline

### Requirement: A conversation records the pipeline that originated it
A Conversation SHALL record the Pipeline that created it. That reference is PROVENANCE: it SHALL be written once at creation and SHALL NOT be read to resolve wiring — the profile, channels and capabilities a conversation runs with come from its own materialized fields, so editing or deleting the originating Pipeline SHALL NOT alter a running conversation.

The reference SHALL scope conversation reuse: a signal MAY only be appended to an existing conversation originated by the SAME Pipeline, so two Pipelines fanning out from one source never share a conversation. A conversation predating the reference SHALL be reusable only while exactly one Ready Pipeline serves the source, and SHALL NOT be backfilled by inference.

Attribution displays SHALL read the recorded origin rather than inferring it from matching bindings.

#### Scenario: Fanned-out conversations do not merge
- **WHEN** a second signal with the same signature arrives on a source two Ready Pipelines serve
- **THEN** each Pipeline's existing conversation receives it, and neither receives the other's

#### Scenario: Origin survives a rewiring
- **WHEN** the originating Pipeline's profile or capability bindings are edited after a conversation exists
- **THEN** the conversation keeps running with the bindings it materialized, and its recorded origin still names that Pipeline

#### Scenario: Attribution is read, not guessed
- **WHEN** two Pipelines with identical bindings each originate a conversation
- **THEN** each conversation is attributed to its own Pipeline rather than left blank as ambiguous
