# signal-source-model — delta

## MODIFIED Requirements

### Requirement: SignalSource CRD splits shared metadata from type-specific config
The `SignalSource` CRD SHALL consist of type-agnostic metadata — required immutable open-string `spec.type`, typed `spec.grouping` (signatureLabels, windowDays, cooldownHours), optional `spec.credentialsSecretRef` — plus an optional opaque `spec.config` (`x-kubernetes-preserve-unknown-fields`) whose shape only the serving signal implementation defines and validates. The operator SHALL never interpret `spec.config` and SHALL never read the credential Secret's values (name-only projection). **The source SHALL carry no wiring: `channelRef` and `profileRef` are removed (BREAKING) — a `Pipeline` claim is the only way a source reaches a profile and channels.**

#### Scenario: Arbitrary config accepted for any type
- **WHEN** a SignalSource is applied with `type: pagerduty` and a `config` object the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Type is required and immutable
- **WHEN** a SignalSource is applied without `spec.type`, or an existing one's `spec.type` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Wiring fields no longer accepted
- **WHEN** a manifest sets `spec.channelRef` or `spec.profileRef` against the new CRD
- **THEN** the fields are pruned/rejected, and the documented migration (a Pipeline claiming the source) provides the routing

#### Scenario: Credentials declared on the source, materialized only by the kubelet
- **WHEN** a SignalSource sets `credentialsSecretRef: {name: pd-api-key}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

## ADDED Requirements

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
