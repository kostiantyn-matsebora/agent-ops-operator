## MODIFIED Requirements

### Requirement: Pipeline-only resolution
Routing SHALL resolve wiring exclusively through Ready Pipelines: a source's signals route via EVERY Ready Pipeline that lists it, and a source no Ready Pipeline lists drops its signals with a visible reason and a `Wired=False` condition. Resolution SHALL NOT fall back to pipeline creation order in any lane: the "oldest claimant" tiebreak is REMOVED, because a source no longer has a single claimant to pick.

This applies uniformly to every source kind, INCLUDING chat sources — a conversation originated by a general-surface chat message resolves through the Pipelines listing that chat `SignalSource`, exactly as an alert resolves through the Pipelines listing its alert source. Chat differs in ONE respect, and only for a message addressing no Pipeline: it is routed only when exactly one Ready Pipeline serves the source, because a person is owed a single answer and can name the Pipeline they want.

A `Channel` SHALL NOT supply a default profile, and inbound resolution SHALL NOT fall back to pipeline creation order: the previous "oldest Ready Pipeline referencing this channel" tiebreak is REMOVED, because origination no longer happens on a channel. Channels remain shareable across Pipelines for delivery and mirroring, unaffected. Thread replies continue to resolve through the conversation's own thread binding, independent of any pipeline lookup.

A `/<name> <task>` chat command SHALL address a PIPELINE by name — the Pipeline originates the conversation, so it supplies the profile AND the capabilities; addressing a profile would name something with no wiring and therefore nothing to grant. The Pipeline listing SHALL enumerate Ready Pipelines rather than profiles, so it advertises only names a user can actually address. No CR other than `Pipeline` carries standing wiring; `Conversation` fields are materialized per-conversation state, not wiring.

**The addressed form SHALL be a SINGLE SEGMENT naming a Pipeline.** A chat command SHALL NOT carry a per-message agent override, and no addressing form SHALL let whoever types it select an agent definition the wiring did not declare. A Pipeline names one profile and a profile names one agent: the agent that runs is therefore fully determined by the wiring, exactly as the toolsets and MCP servers are. A caller choosing its own agent is the same shape as a caller naming its own Pipeline over HTTP, which this capability already forbids — everything runs in a Pipeline, and nothing reaches past one.

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
- **WHEN** a user asks for the Pipeline listing
- **THEN** it names Ready Pipelines, not AgentProfiles — a profile name cannot be addressed

#### Scenario: The agent is determined by the wiring, never by the caller
- **WHEN** a user sends a command carrying a second segment after the Pipeline name
- **THEN** no agent override is applied, and the agent that runs is the one the Pipeline's profile declares

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
