## MODIFIED Requirements

### Requirement: A conversation records the pipeline that originated it
A Conversation SHALL record the Pipeline that created it. That reference is PROVENANCE: written once at creation, never read to resolve wiring.

The profile, channels and capabilities a conversation runs with come from its own materialized fields, so editing or deleting the originating Pipeline SHALL NOT alter a running conversation.

The reference SHALL scope id-equality routing: a signal MAY only be appended to an open conversation originated by the SAME Pipeline, so two Pipelines fanning out from one source never share a conversation even when both adopted the same `initiationId`.

A conversation predating `correlationId` SHALL NOT be matched by equality routing and SHALL NOT be backfilled by inference — a recurrence opens a new conversation beside it.

Attribution displays SHALL read the recorded origin rather than inferring it from matching bindings.

#### Scenario: Fanned-out conversations do not merge
- **WHEN** a second signal with the same `initiationId` arrives on a source two Ready Pipelines serve
- **THEN** each Pipeline's existing conversation receives it, and neither receives the other's

#### Scenario: Origin survives a rewiring
- **WHEN** the originating Pipeline's profile or capability bindings are edited after a conversation exists
- **THEN** the conversation keeps running with the bindings it materialized, and its recorded origin still names that Pipeline

#### Scenario: Attribution is read, not guessed
- **WHEN** two Pipelines with identical bindings each originate a conversation
- **THEN** each conversation is attributed to its own Pipeline rather than left blank as ambiguous
