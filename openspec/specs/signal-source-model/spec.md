# signal-source-model

## Purpose

The restructured `SignalSource` CRD: an open string `spec.adapter` with opaque per-type `config` and name-only credential references, manager-side grouping policy applied uniformly to every source type, and unchanged in-process compatibility for the built-in `alertmanagerWebhook` type.

## Requirements

### Requirement: SignalSource CRD splits shared metadata from type-specific config
The `SignalSource` CRD SHALL consist of type-agnostic metadata — **required immutable `spec.adapter`, naming the `SignalAdapter` whose implementation serves this source and whose schema governs `spec.config`** — plus typed `spec.grouping` (signatureLabels, windowDays, cooldownHours) and optional `spec.credentialsSecretRef`, and an optional opaque `spec.config` (`x-kubernetes-preserve-unknown-fields`) whose shape only that adapter defines and validates. As on `Channel`, selector and config stay siblings and the selector is named for what it references. The operator SHALL never interpret `spec.config` and SHALL never read the credential Secret's values (name-only projection). The source SHALL carry no wiring — a `Pipeline` claim is the only way a source reaches a profile and channels.

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

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

#### Scenario: Adapter-fed signals group manager-side
- **WHEN** two normalized signals with different fingerprints but identical signature labels arrive via an adapter within the source's window
- **THEN** they land in the same conversation, the second as a recurrence when a session exists

#### Scenario: Same-signature batch collapses to one input
- **WHEN** one inbound batch carries several fresh signals sharing a signature
- **THEN** they land as ONE combined input on one conversation
