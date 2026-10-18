# signal-source-model — delta

## ADDED Requirements

### Requirement: Cooldown keys on the correlation id and survives restarts

The manager SHALL suppress a signal whose `initiationId` was admitted on the same source within the source's `spec.cooldownHours`, so at-least-once delivery is safe. A repeat inside the window is a re-send and is dropped. A repeat outside it is a re-notification and routes by id equality.

Dedup granularity therefore matches the sender's own re-notification policy. `cooldownHours` is sized below the sender's re-notification interval, so a genuine re-notification appends as a recurrence rather than being absorbed.

Cooldown state SHALL be durable rather than process memory:

- suppression is recorded on the owning `SignalSource`, keyed by `initiationId`
- the record is loaded before cooldown is applied to that source after a restart
- an in-memory map MAY remain the hot path, but SHALL NOT be the record
- entries past their window SHALL be pruned so the object stays bounded

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with an `initiationId` seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

#### Scenario: Cooldown survives a manager restart
- **WHEN** the manager restarts and an adapter re-delivers a signal whose `initiationId` was suppressed before the restart and is still inside `cooldownHours`
- **THEN** the signal is still suppressed and no duplicate conversation or input is created

#### Scenario: A re-notification outside the window appends
- **WHEN** a sender re-notifies with the same `initiationId` after `cooldownHours`, while the adopted conversation is still open
- **THEN** the signal is appended to that conversation as a recurrence

#### Scenario: Same-id batch collapses to one input
- **WHEN** one inbound batch carries several fresh signals sharing an `initiationId`
- **THEN** they land as ONE combined input on one conversation

#### Scenario: Suppression record stays bounded
- **WHEN** recorded suppression entries age past their cooldown window
- **THEN** they are pruned from the `SignalSource` rather than accumulating

## MODIFIED Requirements

### Requirement: SignalSource CRD splits shared metadata from type-specific config
The `SignalSource` CRD SHALL consist of type-agnostic metadata plus an optional opaque `spec.config`:

- required immutable `spec.adapter` — the name of the `SignalAdapter` CR serving this source, a REFERENCE whose implementation defines and validates the sibling config (it replaced the open-string `spec.type`)
- typed `spec.cooldownHours` — how long a repeat of one `initiationId` is suppressed
- optional `spec.credentialsSecretRef`
- optional opaque `spec.config` (`x-kubernetes-preserve-unknown-fields`), whose shape only the serving signal implementation defines and validates authoritatively

The source SHALL carry no grouping policy: `spec.grouping` (`signatureLabels`, `windowDays`, `cooldownHours`) is removed (BREAKING). Which signals are one problem is decided where the `initiationId` is minted, and the manager routes by id equality alone.

The operator SHALL never interpret `spec.config` semantically and SHALL never read the credential Secret's values (name-only projection).

**When the serving `SignalAdapter` CR declares a config schema, the manager SHALL mechanically validate `spec.config` against that adapter-declared schema and report the result as an advisory `ConfigValid` condition — admission still accepts arbitrary config, and a violation never blocks serving or ingestion.**

**The source SHALL carry no wiring: `channelRef` and `profileRef` are removed (BREAKING) — a `Pipeline` claim is the only way a source reaches a profile and channels.**

#### Scenario: Wiring fields no longer accepted
- **WHEN** a manifest sets `spec.channelRef` or `spec.profileRef` against the new CRD
- **THEN** the fields are pruned/rejected, and the documented migration (a Pipeline claiming the source) provides the routing

#### Scenario: The retired grouping stanza is pruned
- **WHEN** a manifest sets `spec.grouping` against the new CRD
- **THEN** the field is not part of the schema and is pruned rather than honoured, so no source carries grouping policy

#### Scenario: Credentials declared on the source, materialized only by the kubelet
- **WHEN** a SignalSource sets `credentialsSecretRef: {name: pd-api-key}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

#### Scenario: Arbitrary config accepted for any adapter
- **WHEN** a SignalSource is applied with `adapter: pagerduty` and a `config` object the operator has never seen, and no SignalAdapter declares a schema
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation, and no `ConfigValid` condition is set

#### Scenario: Adapter reference is required and immutable
- **WHEN** a SignalSource is applied without `spec.adapter`, or an existing one's `spec.adapter` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Schema violation surfaces as an advisory condition
- **WHEN** a SignalSource's `config` violates the schema its serving SignalAdapter declares
- **THEN** the API server still accepts the SignalSource, its status gains `ConfigValid=False` naming the violation, and `Served`/`Wired`/signal ingestion are unaffected

#### Scenario: The former type field is gone
- **WHEN** a SignalSource is applied carrying the removed `spec.type`
- **THEN** the field is not part of the schema and is pruned rather than honoured, so a stale manifest cannot silently select an adapter

## REMOVED Requirements

### Requirement: Grouping policy stays manager-side for every source type

**Reason**: The manager holds no grouping policy any more. "These N signals are one problem" is decided wherever the `initiationId` is minted, and the manager routes by pure id equality. Signature grouping and fingerprint cooldown go with `spec.grouping`.

**Migration**: Every adapter mints `initiationId` where its aggregation already happens. Cooldown keys on `initiationId` from `spec.cooldownHours`, durable per the added requirement. Window reuse is replaced by equality against OPEN conversations — a recurrence after close opens a new one.
