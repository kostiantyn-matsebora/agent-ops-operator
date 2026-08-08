## MODIFIED Requirements

### Requirement: Pipeline-only resolution
Routing SHALL resolve wiring exclusively through Ready Pipelines: a source's signals route via the pipeline that claims it (unclaimed sources drop signals with a visible reason and a `Wired=False` condition). This applies uniformly to every source kind, INCLUDING chat sources — a conversation originated by a general-surface chat message resolves through the Pipeline claiming that chat `SignalSource`, exactly as an alert resolves through the Pipeline claiming its alert source.

A `Channel` SHALL NOT supply a default profile, and inbound resolution SHALL NOT fall back to pipeline creation order: the previous "oldest Ready Pipeline referencing this channel" tiebreak is REMOVED, because origination no longer happens on a channel. Channels remain shareable across Pipelines for delivery and mirroring, unaffected. Thread replies continue to resolve through the conversation's own thread binding, independent of any pipeline lookup. No CR other than `Pipeline` carries standing wiring; `Conversation` fields are materialized per-conversation state, not wiring.

#### Scenario: Unclaimed source routes nothing
- **WHEN** a signal arrives for a source no Ready Pipeline references
- **THEN** no conversation is created, the response says so, and the source shows `Wired=False`

#### Scenario: Unclaimed chat source routes nothing
- **WHEN** a general-surface chat message arrives for a chat source no Ready Pipeline claims
- **THEN** no conversation is created, the source shows `Wired=False`, and the drop reason reaches the originating surface

#### Scenario: Commands work through the chat source
- **WHEN** a user sends `/some-profile do it` on a channel whose chat source is claimed
- **THEN** a conversation is created for that profile through the signal path

#### Scenario: Bare message resolves through the claiming pipeline
- **WHEN** a non-command message arrives on a channel's general surface
- **THEN** the conversation uses the profile of the Pipeline claiming the chat source and is bound to all that Pipeline's channels

#### Scenario: Shared channels need no tiebreak
- **WHEN** two Ready Pipelines both reference channel `web` for delivery
- **THEN** neither ordering nor creation timestamps affect inbound resolution, because neither claims origination on it

#### Scenario: Replies bypass pipeline resolution entirely
- **WHEN** a user replies inside an existing thread
- **THEN** the input is appended to that thread's conversation with no pipeline lookup
