# pipeline-model — delta

## RENAMED Requirements

- FROM: `### Requirement: Pipeline-first resolution with source-level fallback`
- TO: `### Requirement: Pipeline-only resolution`

## MODIFIED Requirements

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
