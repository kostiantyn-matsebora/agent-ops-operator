# signal-source-model

## Purpose

The `SignalSource` CRD: `spec.adapter` naming the serving `SignalAdapter`, opaque per-adapter `config`, name-only credential references, and manager-side cooldown keyed on the sender-minted `initiationId`, applied uniformly to every source. There are no built-in signal types — the manager hosts no signal transport, so every source needs a serving adapter.

## Requirements

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

### Requirement: Unwired sources are visible and drop signals loudly
A SignalSource SHALL carry a `Wired` condition: True when at least one Ready `Pipeline` lists it, False otherwise. The condition SHALL name ALL the Pipelines serving it, not the first — a source several Pipelines watch fans its signals out to every one of them, so "who answers here" is only readable if the condition says all of them. Signals arriving for an unwired source SHALL NOT create conversations; the ingest/inbound response SHALL state the reason explicitly.

The NUMBER of serving Ready Pipelines is what an operator needs to predict behaviour, and it means two different things by lane: for any source it is the number of conversations one signal will produce, and for a CHAT source it additionally decides bare-message behaviour — one makes a bare message unambiguous and routable, several make it ambiguous and answerable with the choices. An operator SHALL be able to read that number from the condition rather than by listing Pipelines and matching refs by hand.

#### Scenario: Unwired source reports itself
- **WHEN** a SignalSource exists that no Ready Pipeline references
- **THEN** its status shows `Wired=False`

#### Scenario: Signals for an unwired source are dropped with a reason
- **WHEN** a signal arrives for an unwired source
- **THEN** no conversation or input is created and the response carries queued 0 with an explicit not-wired reason

#### Scenario: Claim flips the condition
- **WHEN** a Ready Pipeline adds the source to its `signalSourceRefs`
- **THEN** the source's `Wired` condition becomes True naming that pipeline and subsequent signals route

#### Scenario: A source served by several pipelines names them all
- **WHEN** two Ready Pipelines list one source
- **THEN** the source reports `Wired=True` naming both, which is also what tells an operator that one signal there will open two conversations

#### Scenario: A chat surface served by several pipelines names them all
- **WHEN** two Ready Pipelines list one chat source
- **THEN** the source reports `Wired=True` naming both, which is also what tells an operator that bare messages there will be refused as ambiguous

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

### Requirement: SignalSource may filter by signal attribute before routing

A `SignalSource` MAY declare `spec.attributeFilters`, a list of
requirements shaped like a Kubernetes `matchExpressions` entry:

| Field | Holds |
|---|---|
| `key` | the attribute label name to test |
| `operator` | `In`, `NotIn`, `Exists`, or `DoesNotExist` |
| `values` | a string list, required for `In`/`NotIn`, empty for `Exists`/`DoesNotExist` |

An unknown or malformed `operator` SHALL be rejected at the API level, not
silently ignored at ingest time.

Each requirement is evaluated against the normalized signal's attribute
labels, with the same matching semantics a Kubernetes label selector
requirement applies.

A signal SHALL be admitted only when it satisfies EVERY requirement in
the list. Failing any one of them drops the signal.

An empty or absent `attributeFilters` SHALL admit every signal. That is
identical to today's behavior with no filter declared at all.

Filtering SHALL be evaluated before cooldown is consulted or spent, and
before the source's `Wired` pipelines are routed to. A signal the filter
drops:

- SHALL NOT count toward cooldown suppression
- SHALL NOT reach any serving pipeline
- SHALL NOT be counted as received by the source

#### Scenario: A signal matching every requirement routes normally

- **WHEN** a SignalSource declares `attributeFilters` and a signal arrives
  whose labels satisfy every requirement in the list
- **THEN** the signal proceeds to cooldown and routing exactly as it would
  with no filter declared

#### Scenario: A signal failing one requirement is dropped before cooldown

- **WHEN** a signal arrives whose labels fail at least one requirement in
  the source's `attributeFilters`
- **THEN** the signal is dropped before cooldown is consulted, consumes no
  cooldown state, and never reaches a serving pipeline

#### Scenario: Empty or absent attributeFilters admits everything

- **WHEN** a SignalSource sets `attributeFilters` to an empty list, or
  omits the field entirely
- **THEN** every signal is admitted to cooldown and routing unchanged, as
  if no filter existed

#### Scenario: An unknown operator is rejected at creation

- **WHEN** a SignalSource is applied with an `attributeFilters` entry whose
  `operator` is not one of `In`, `NotIn`, `Exists`, or `DoesNotExist`
- **THEN** the API server rejects the request with a validation error
  rather than accepting it and ignoring the requirement at ingest time

### Requirement: SignalSource controls whether a filtered drop is recorded

A `SignalSource` MAY declare `spec.logFiltered`, a nullable boolean. When
unset or null, it behaves as `true`.

It governs ONLY whether a signal dropped by `attributeFilters` is
recorded as a dropped-signal activity event. It SHALL have no effect on
whether the signal itself is dropped.

It SHALL have no effect on the recording of any OTHER drop reason. An
unwired source, an at-capacity backlog, or an ambiguous bare chat message
SHALL always be recorded regardless of this field's value.

#### Scenario: logFiltered false suppresses only the filtered-drop record

- **WHEN** a SignalSource sets `logFiltered: false` and a signal is
  dropped by `attributeFilters`
- **THEN** no dropped-signal activity event is recorded for that signal,
  though the signal is still dropped

#### Scenario: Other drop reasons still log regardless of logFiltered

- **WHEN** a SignalSource sets `logFiltered: false` and a separate signal
  on the same source is dropped for being unwired or at capacity
- **THEN** a dropped-signal activity event is still recorded for that
  signal

#### Scenario: logFiltered unset behaves as true

- **WHEN** a SignalSource omits `logFiltered` (or sets it to null) and a
  signal is dropped by `attributeFilters`
- **THEN** a dropped-signal activity event is recorded for that signal,
  the same as if `logFiltered: true` had been set explicitly
