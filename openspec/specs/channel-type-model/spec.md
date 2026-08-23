# channel-type-model

## Purpose

The generic Channel CRD: type-agnostic metadata plus opaque per-adapter config, string thread ids, and the operator's exclusive ownership of delivery.

## Requirements

### Requirement: Channel CRD splits shared metadata from type-specific config
The `Channel` CRD SHALL consist of type-agnostic metadata — required immutable `spec.adapter` (the name of the `ChannelAdapter` CR serving this surface, a REFERENCE whose implementation defines and validates the sibling config), optional `spec.credentialsSecretRef` (LocalObjectReference naming the Secret holding this surface's transport credentials — credentials are per-surface usage, never per-implementation) — plus an optional opaque `spec.config` object (`x-kubernetes-preserve-unknown-fields`) whose shape is defined and validated authoritatively by the serving adapter. Those three fields SHALL be the whole of `spec`. The operator SHALL never interpret `spec.config` semantically and SHALL never read the referenced credential Secret's values (it only projects the secret *name* into adapter pod specs). **When the serving `ChannelAdapter` CR declares a config schema, the manager SHALL mechanically validate `spec.config` against that adapter-declared schema and report the result as an advisory `ConfigValid` condition — admission still accepts arbitrary config, and a violation never blocks serving.** **The channel SHALL carry no wiring and no delivery hints:** `defaultProfileRef`, `spec.type`, `spec.delivery` and the typed `spec.telegram` sub-struct are all removed (**BREAKING**; migration documented).

#### Scenario: A channel supplies no default profile at all
- **WHEN** a bare (non-command) message arrives on a chat surface
- **THEN** the Channel is not consulted for a profile: the message arrives as a `kind: chat` signal from a chat `SignalSource`, and the Pipelines CLAIMING THAT SOURCE decide who answers — one claimant routes, several are answered with the choices and the `/<pipeline> <task>` form, none is the unwired drop
- **AND** nothing resolves a "default profile" from the channel's Pipelines by creation order; that tiebreak was removed with `defaultProfileRef` and was not replaced by another one

#### Scenario: Credentials declared on the surface, materialized only by the kubelet
- **WHEN** a Channel sets `credentialsSecretRef: {name: ops-bot}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

#### Scenario: Arbitrary config accepted for any adapter
- **WHEN** a Channel is applied with `adapter: slack` and a `config` object with fields the operator has never seen, and no ChannelAdapter declares a schema
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation, and no `ConfigValid` condition is set

#### Scenario: Adapter reference is required and immutable
- **WHEN** a Channel is applied without `spec.adapter`, or an existing Channel's `spec.adapter` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: The former type and delivery fields are gone
- **WHEN** a Channel is applied carrying either removed field — `spec.type` or `spec.delivery`
- **THEN** neither is part of the schema and both are pruned rather than honoured, so a stale manifest cannot silently select an adapter or influence delivery

#### Scenario: Schema violation surfaces as an advisory condition
- **WHEN** a Channel's `config` violates the schema the serving ChannelAdapter declares
- **THEN** the API server still accepts the Channel, its status gains `ConfigValid=False` naming the violation, and `Served`/delivery are unaffected

#### Scenario: Old shape no longer served
- **WHEN** a manifest using the removed `spec.telegram` sub-struct is applied against the new CRD
- **THEN** validation fails, and the documented migration (telegram sub-struct → `adapter: telegram` + `config`) produces an accepted equivalent

### Requirement: Conversation thread ids are strings
A conversation's thread id SHALL be a string, carried as a string through the dispatch `WorkUnit` and the runtime environment (**BREAKING** for numeric consumers). It lives on `ConversationStatus.threads[].threadId`, one binding per bound channel — the single `status.threadId` it replaced could hold only one surface's thread. Channel implementations define their own id format; conversation-to-thread resolution SHALL compare ids as opaque strings scoped to the Channel.

#### Scenario: Non-numeric thread id round-trips
- **WHEN** a channel implementation completes topic creation with thread id `"1234567890.000200"`
- **THEN** the id is stored on that channel's `status.threads[]` binding verbatim, appears in dispatched work units, and an inbound message carrying it resolves to that conversation

#### Scenario: Pre-migration numeric ids stay valid
- **WHEN** a Conversation whose thread id was written as a number is migrated
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
