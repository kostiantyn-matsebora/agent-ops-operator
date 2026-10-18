# pipeline-model — delta

## MODIFIED Requirements

### Requirement: Sources are shareable and signals fan out
A `SignalSource` MAY be listed by any number of Ready Pipelines, of any signal kind. Doing so SHALL NOT produce a conflict condition and SHALL NOT affect any Pipeline's `Ready`.

Listing a source means "I watch this" — it makes the source wired and, on a chat surface, makes the Pipeline addressable there — not "I own this".

A signal admitted on a source served by several Ready Pipelines SHALL produce one conversation PER Pipeline, each carrying that Pipeline's own profile, channel set and capabilities.

Per-source ingest policy — cooldown on `initiationId` — SHALL be evaluated ONCE, before the fan-out. An id is admitted once and then delivered to each server, rather than being suppressed for all but the first. Id-equality routing then runs per Pipeline, against that Pipeline's own conversations.

Channels MAY likewise be referenced by multiple Pipelines. A conversation's binding set comes from the Pipeline that originated it.

#### Scenario: Two pipelines watching one alert source both investigate
- **WHEN** an alert arrives on a source two Ready Pipelines list
- **THEN** two conversations are created, one per Pipeline, each with its own profile and capabilities, and neither Pipeline reports a conflict

#### Scenario: Cooldown is not spent by the first server
- **WHEN** an `initiationId` is admitted on a source two Ready Pipelines list
- **THEN** both Pipelines receive it, and the id is recorded as admitted once for the source

#### Scenario: Several pipelines serve one chat surface
- **WHEN** two Ready Pipelines both list the same chat SignalSource
- **THEN** neither reports a conflict, both stay `Ready=True`, and both appear in the `/pipelines` listing on that surface

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline

### Requirement: A conversation records the pipeline that originated it
A Conversation SHALL record the Pipeline that created it. That reference is PROVENANCE: written once at creation, never read to resolve wiring.

The profile, channels and capabilities a conversation runs with come from its own materialized fields, so editing or deleting the originating Pipeline SHALL NOT alter a running conversation.

The reference SHALL scope id-equality routing: a signal MAY only be appended to an open conversation originated by the SAME Pipeline, so two Pipelines fanning out from one source never share a conversation even when both adopted the same `initiationId`.

A conversation predating `conversationId` SHALL NOT be matched by equality routing and SHALL NOT be backfilled by inference — a recurrence opens a new conversation beside it.

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
