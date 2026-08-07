# channel-type-model

## Purpose

The generic Channel CRD: type-agnostic metadata plus opaque per-type config, string thread ids, and metadata-driven delivery instructions.

## Requirements

### Requirement: Channel CRD splits shared metadata from type-specific config
The `Channel` CRD SHALL consist of type-agnostic metadata — required immutable `spec.type` (string channel-type identifier), optional `spec.delivery` hints, optional `spec.credentialsSecretRef` (LocalObjectReference naming the Secret holding this surface's transport credentials — credentials are per-surface usage, never per-implementation) — plus an optional opaque `spec.config` object (`x-kubernetes-preserve-unknown-fields`) whose shape is defined and validated authoritatively by the channel-type implementation. The operator SHALL never interpret `spec.config` semantically and SHALL never read the referenced credential Secret's values (it only projects the secret *name* into adapter pod specs). **When the serving `ChannelAdapter` CR declares a config schema for the type, the manager SHALL mechanically validate `spec.config` against that adapter-declared schema and report the result as an advisory `ConfigValid` condition — admission still accepts arbitrary config, and a violation never blocks serving.** **The channel SHALL carry no wiring: `defaultProfileRef` is removed (BREAKING) — a channel's default profile for bare messages comes from its oldest Ready `Pipeline`.** The typed `spec.telegram` sub-struct SHALL be removed (**BREAKING**; migration documented).

#### Scenario: Default profile comes from the pipeline
- **WHEN** a bare (non-command) message arrives on a channel referenced by a Ready Pipeline
- **THEN** the conversation uses the pipeline's profile; on a channel in no pipeline, the user gets the "no default profile" guidance message

#### Scenario: Credentials declared on the surface, materialized only by the kubelet
- **WHEN** a Channel sets `credentialsSecretRef: {name: ops-bot}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

#### Scenario: Arbitrary config accepted for any adapter
- **WHEN** a Channel is applied with `adapter: slack` and a `config` object with fields the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Adapter reference is required and immutable
- **WHEN** a Channel is applied without `spec.adapter`, or an existing Channel's `spec.adapter` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: The former type field is gone
- **WHEN** a Channel is applied carrying `spec.type`
- **THEN** the field is not part of the schema and is pruned rather than honoured, so a stale manifest cannot silently select an adapter

#### Scenario: Arbitrary config accepted for any type
- **WHEN** a Channel is applied with `type: slack` and a `config` object with fields the operator has never seen, and no ChannelAdapter declares a schema for `slack`
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation, and no `ConfigValid` condition is set

#### Scenario: Schema violation surfaces as an advisory condition
- **WHEN** a Channel's `config` violates the schema the serving ChannelAdapter declares
- **THEN** the API server still accepts the Channel, its status gains `ConfigValid=False` naming the violation, and `Served`/delivery are unaffected

#### Scenario: Type is required and immutable
- **WHEN** a Channel is applied without `spec.type`, or an existing Channel's `spec.type` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Old shape no longer served
- **WHEN** a manifest using the removed `spec.telegram` sub-struct is applied against the new CRD
- **THEN** validation fails, and the documented migration (telegram sub-struct → `type: telegram` + `config`) produces an accepted equivalent

### Requirement: Conversation thread ids are strings
`ConversationStatus.threadId` SHALL be a string, carried as a string through the dispatch `WorkUnit` and the runtime environment (**BREAKING** for numeric consumers). Channel implementations define their own id format; conversation-to-thread resolution SHALL compare ids as opaque strings scoped to the Channel.

#### Scenario: Non-numeric thread id round-trips
- **WHEN** a channel implementation completes topic creation with thread id `"1234567890.000200"`
- **THEN** the id is stored in `status.threadId` verbatim, appears in dispatched work units, and an inbound message carrying it resolves to that conversation

#### Scenario: Pre-migration numeric ids stay valid
- **WHEN** a Conversation whose `threadId` was written as a number is migrated
- **THEN** its decimal string form resolves exactly as before

### Requirement: Delivery is the operator's, and prompts stay transport-blind
Dispatch SHALL contain no channel-specific delivery wording, and a `Channel`
SHALL offer no way to supply any: the delivery section of every prompt is
invariant and states that the agent's printed answer IS the deliverable,
captured via `/work/done`. The manager SHALL post that result to every bound
thread through the serving adapters — for single- and multi-channel
conversations alike. Consequently no agent learns a transport and no runtime
holds a channel's credentials.

#### Scenario: Delivery wording cannot be influenced by a channel
- **WHEN** a work unit is dispatched for a conversation on any channel
- **THEN** its prompt carries the printed-answer instructions, forbids sending chat messages directly, and contains no transport-specific steps

#### Scenario: Single-channel results are posted by the operator
- **WHEN** a run completes on a conversation bound to exactly one channel
- **THEN** the manager enqueues a `send` op carrying the result to that channel's thread, rather than relying on the agent to post it
