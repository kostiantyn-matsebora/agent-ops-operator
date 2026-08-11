# signal-source-model

## Purpose

The restructured `SignalSource` CRD: an open string `spec.adapter` with opaque per-type `config` and name-only credential references, manager-side grouping policy applied uniformly to every source type, and unchanged in-process compatibility for the built-in `alertmanagerWebhook` type.
## Requirements
### Requirement: SignalSource CRD splits shared metadata from type-specific config
The `SignalSource` CRD SHALL consist of type-agnostic metadata — required immutable open-string `spec.type`, typed `spec.grouping` (signatureLabels, windowDays, cooldownHours), optional `spec.credentialsSecretRef` — plus an optional opaque `spec.config` (`x-kubernetes-preserve-unknown-fields`) whose shape only the serving signal implementation defines and validates authoritatively. The operator SHALL never interpret `spec.config` semantically and SHALL never read the credential Secret's values (name-only projection). **When the serving `SignalAdapter` CR declares a config schema for the type, the manager SHALL mechanically validate `spec.config` against that adapter-declared schema and report the result as an advisory `ConfigValid` condition — admission still accepts arbitrary config, and a violation never blocks serving or ingestion.** **The source SHALL carry no wiring: `channelRef` and `profileRef` are removed (BREAKING) — a `Pipeline` claim is the only way a source reaches a profile and channels.**

#### Scenario: Wiring fields no longer accepted
- **WHEN** a manifest sets `spec.channelRef` or `spec.profileRef` against the new CRD
- **THEN** the fields are pruned/rejected, and the documented migration (a Pipeline claiming the source) provides the routing

#### Scenario: Credentials declared on the source, materialized only by the kubelet
- **WHEN** a SignalSource sets `credentialsSecretRef: {name: pd-api-key}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

#### Scenario: Arbitrary config accepted for any adapter
- **WHEN** a SignalSource is applied with `adapter: pagerduty` and a `config` object the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Adapter reference is required and immutable
- **WHEN** a SignalSource is applied without `spec.adapter`, or an existing one's `spec.adapter` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Arbitrary config accepted for any type
- **WHEN** a SignalSource is applied with `type: pagerduty` and a `config` object the operator has never seen, and no SignalAdapter declares a schema for `pagerduty`
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation, and no `ConfigValid` condition is set

#### Scenario: Schema violation surfaces as an advisory condition
- **WHEN** a SignalSource's `config` violates the schema its serving SignalAdapter declares
- **THEN** the API server still accepts the SignalSource, its status gains `ConfigValid=False` naming the violation, and `Served`/`Wired`/signal ingestion are unaffected

#### Scenario: Type is required and immutable
- **WHEN** a SignalSource is applied without `spec.type`, or an existing one's `spec.type` is changed
- **THEN** the API server rejects the request with a validation error

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

### Requirement: Grouping policy stays manager-side for every source type
Signature grouping, fingerprint cooldown, window-based conversation reuse, and recurrence-on-session SHALL be applied by the manager from `spec.grouping` for signals of every source type. Adapters SHALL NOT need to implement any grouping logic.

Cooldown state SHALL be durable rather than process memory: the manager SHALL record fingerprint suppression on the owning `SignalSource` and SHALL load it before applying cooldown to that source after a restart, so a restart does not re-open conversations for signals inside an active window. An in-memory map MAY remain the hot path, but SHALL NOT be the record. Recorded entries SHALL be pruned once past their window so the object stays bounded.

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

#### Scenario: Cooldown survives a manager restart
- **WHEN** the manager restarts and an adapter re-delivers a signal whose fingerprint was suppressed before the restart and is still inside `cooldownHours`
- **THEN** the signal is still suppressed and no duplicate conversation is opened

#### Scenario: Adapter-fed signals group manager-side
- **WHEN** two normalized signals with different fingerprints but identical signature labels arrive via an adapter within the source's window
- **THEN** they land in the same conversation, the second as a recurrence when a session exists

#### Scenario: Same-signature batch collapses to one input
- **WHEN** one inbound batch carries several fresh signals sharing a signature
- **THEN** they land as ONE combined input on one conversation

#### Scenario: Alerts keep the default labels when a source declares none
- **WHEN** two `kind: alert` signals with different fingerprints but the same `alertname` and `namespace` arrive at a source with `grouping: {}`
- **THEN** they land in the same conversation, exactly as before — the default labels still apply to the alert lane

#### Scenario: Recurring jobs keep folding into one conversation
- **WHEN** a job source with no `signatureLabels` fires successive ticks carrying distinct fingerprints
- **THEN** the ticks land in the same conversation and later ones resume the agent session as recurrences

#### Scenario: One-shot lanes key on the fingerprint
- **WHEN** two `kind: task` signals with different fingerprints arrive at a source with no `signatureLabels`
- **THEN** each opens its own conversation, rather than sharing the empty default signature

#### Scenario: Explicit labels override the lane default
- **WHEN** a source declares `signatureLabels` and receives `kind: task` signals sharing those label values
- **THEN** they group under that signature — an operator who asks for grouping gets it in every lane

#### Scenario: Suppression record stays bounded
- **WHEN** recorded suppression entries age past their cooldown window
- **THEN** they are pruned from the `SignalSource` rather than accumulating

