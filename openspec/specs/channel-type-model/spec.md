# channel-type-model

## Purpose

The generic Channel CRD: type-agnostic metadata plus opaque per-type config, string thread ids, and metadata-driven delivery instructions.

## Requirements

### Requirement: Channel CRD splits shared metadata from type-specific config
The `Channel` CRD SHALL consist of type-agnostic metadata — required immutable `spec.type` (string channel-type identifier), optional `spec.delivery` hints, optional `spec.credentialsSecretRef` (LocalObjectReference naming the Secret holding this surface's transport credentials — credentials are per-surface usage, never per-implementation) — plus an optional opaque `spec.config` object (`x-kubernetes-preserve-unknown-fields`) whose shape is defined and validated solely by the channel-type implementation. The operator SHALL never interpret `spec.config` and SHALL never read the referenced credential Secret's values (it only projects the secret *name* into adapter pod specs). **The channel SHALL carry no wiring: `defaultProfileRef` is removed (BREAKING) — a channel's default profile for bare messages comes from its oldest Ready `Pipeline`.** The typed `spec.telegram` sub-struct SHALL be removed (**BREAKING**; migration documented).

#### Scenario: Arbitrary config accepted for any type
- **WHEN** a Channel is applied with `type: slack` and a `config` object with fields the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Type is required and immutable
- **WHEN** a Channel is applied without `spec.type`, or an existing Channel's `spec.type` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Default profile comes from the pipeline
- **WHEN** a bare (non-command) message arrives on a channel referenced by a Ready Pipeline
- **THEN** the conversation uses the pipeline's profile; on a channel in no pipeline, the user gets the "no default profile" guidance message

#### Scenario: Old shape no longer served
- **WHEN** a manifest using the removed `spec.telegram` sub-struct is applied against the new CRD
- **THEN** validation fails, and the documented migration (telegram sub-struct → `type: telegram` + `config`) produces an accepted equivalent

#### Scenario: Credentials declared on the surface, materialized only by the kubelet
- **WHEN** a Channel sets `credentialsSecretRef: {name: ops-bot}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

### Requirement: Conversation thread ids are strings
`ConversationStatus.threadId` SHALL be a string, carried as a string through the dispatch `WorkUnit` and the runtime environment (**BREAKING** for numeric consumers). Channel implementations define their own id format; conversation-to-thread resolution SHALL compare ids as opaque strings scoped to the Channel.

#### Scenario: Non-numeric thread id round-trips
- **WHEN** a channel implementation completes topic creation with thread id `"1234567890.000200"`
- **THEN** the id is stored in `status.threadId` verbatim, appears in dispatched work units, and an inbound message carrying it resolves to that conversation

#### Scenario: Pre-migration numeric ids stay valid
- **WHEN** a Conversation whose `threadId` was written as a number is migrated
- **THEN** its decimal string form resolves exactly as before

### Requirement: Delivery instructions selected from channel metadata
Dispatch SHALL contain no channel-type-specific delivery wording. Work-unit delivery instructions SHALL be selected from `spec.delivery`: mode `result` (the default, also used for chat-less conversations) instructs that the printed answer is the deliverable captured via `/work/done`; mode `agent` injects the channel's `delivery.agentInstructions` text verbatim. **Conversations bound to more than one channel SHALL force `result` mode regardless of any bound channel's `delivery.mode` — the manager owns distribution on mirrored conversations; `agent` mode retains its meaning only for single-channel conversations.**

#### Scenario: Default mode needs no channel knowledge
- **WHEN** a work unit is dispatched for a conversation on a channel without `spec.delivery`
- **THEN** its prompt carries the printed-answer delivery instructions and no transport-specific steps

#### Scenario: Agent-direct channel supplies its own wording
- **WHEN** a single-channel conversation's channel sets `delivery.mode: agent` with instruction text (e.g., the Telegram curl recipe)
- **THEN** dispatched prompts for its conversations contain exactly that text as the delivery section

#### Scenario: Multi-channel conversations always use result delivery
- **WHEN** a work unit is dispatched for a conversation bound to two channels, one of which sets `delivery.mode: agent`
- **THEN** the prompt carries the printed-answer instructions (no agent-direct steps) and the manager fans the result out to both channels
