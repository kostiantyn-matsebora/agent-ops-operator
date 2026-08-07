# channel-type-model

## Purpose

The generic Channel CRD: type-agnostic metadata plus opaque per-type config, string thread ids, and metadata-driven delivery instructions.

## Requirements

### Requirement: Channel CRD splits shared metadata from type-specific config
The `Channel` CRD SHALL consist of type-agnostic metadata — **required immutable `spec.adapter`, naming the `ChannelAdapter` whose implementation serves this surface and whose schema governs `spec.config`** — plus optional `spec.credentialsSecretRef` (LocalObjectReference naming the Secret holding this surface's transport credentials, per-surface usage, never per-implementation) and an optional opaque `spec.config` object (`x-kubernetes-preserve-unknown-fields`) whose shape is defined and validated solely by that adapter. `spec.adapter` and `spec.config` stay siblings, matching how Kubernetes pairs a selector with its implementation-owned config (`StorageClass.provisioner`/`parameters`, `IngressClass.controller`/`parameters`); the name says the value is a REFERENCE, so `config` is never mistaken for part of one flat schema the selector belongs to. The operator SHALL never interpret `spec.config` and SHALL never read the referenced credential Secret's values (it only projects the secret *name* into adapter pod specs). The channel SHALL carry no wiring and no delivery mode — a channel's default profile for bare messages comes from its oldest Ready `Pipeline`, and the operator delivers all agent output.

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

