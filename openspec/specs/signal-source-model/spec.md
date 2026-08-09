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
A SignalSource SHALL carry a `Wired` condition: True (naming the pipeline) when a Ready `Pipeline` claims it, False otherwise. Signals arriving for an unwired source SHALL NOT create conversations; the ingest/inbound response SHALL state the reason explicitly.

#### Scenario: Unwired source reports itself
- **WHEN** a SignalSource exists that no Ready Pipeline references
- **THEN** its status shows `Wired=False`

#### Scenario: Signals for an unwired source are dropped with a reason
- **WHEN** a signal arrives for an unwired source
- **THEN** no conversation or input is created and the response carries queued 0 with an explicit not-wired reason

#### Scenario: Claim flips the condition
- **WHEN** a Ready Pipeline adds the source to its `signalSourceRefs`
- **THEN** the source's `Wired` condition becomes True naming that pipeline and subsequent signals route

### Requirement: Grouping policy stays manager-side for every source type
Signature grouping, fingerprint cooldown, window-based conversation reuse, and recurrence-on-session SHALL be applied by the manager from `spec.grouping` for signals of every source type. Adapters SHALL NOT need to implement any grouping logic.

When a source declares `signatureLabels`, they SHALL compose the signature for signals of every kind. When a source declares NONE, the fallback SHALL depend on what the lane is about: `alert` and `job` are recurring-subject lanes, where a later signal is more news about the same thing, and SHALL fall back to the default alert labels (`alertgroup`/`alertname`/`namespace`) so they group and resume an existing session; `task` and `chat` are one-shot lanes, where a later signal is a separate request, and SHALL key on the signal's own fingerprint so each opens its own conversation. The default labels are alert vocabulary, so applying them to a one-shot lane would hash every request to one empty signature and pile unrelated work into a single conversation.

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

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
