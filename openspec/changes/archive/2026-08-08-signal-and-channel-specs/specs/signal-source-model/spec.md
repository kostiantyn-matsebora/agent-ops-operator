# signal-source-model — delta

## MODIFIED Requirements

### Requirement: SignalSource CRD splits shared metadata from type-specific config
The `SignalSource` CRD SHALL consist of type-agnostic metadata — required immutable open-string `spec.type`, typed `spec.grouping` (signatureLabels, windowDays, cooldownHours), optional `spec.credentialsSecretRef` — plus an optional opaque `spec.config` (`x-kubernetes-preserve-unknown-fields`) whose shape only the serving signal implementation defines and validates authoritatively. The operator SHALL never interpret `spec.config` semantically and SHALL never read the credential Secret's values (name-only projection). **When the serving `SignalAdapter` CR declares a config schema for the type, the manager SHALL mechanically validate `spec.config` against that adapter-declared schema and report the result as an advisory `ConfigValid` condition — admission still accepts arbitrary config, and a violation never blocks serving or ingestion.** **The source SHALL carry no wiring: `channelRef` and `profileRef` are removed (BREAKING) — a `Pipeline` claim is the only way a source reaches a profile and channels.**

#### Scenario: Arbitrary config accepted for any type
- **WHEN** a SignalSource is applied with `type: pagerduty` and a `config` object the operator has never seen, and no SignalAdapter declares a schema for `pagerduty`
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation, and no `ConfigValid` condition is set

#### Scenario: Schema violation surfaces as an advisory condition
- **WHEN** a SignalSource's `config` violates the schema its serving SignalAdapter declares
- **THEN** the API server still accepts the SignalSource, its status gains `ConfigValid=False` naming the violation, and `Served`/`Wired`/signal ingestion are unaffected

#### Scenario: Type is required and immutable
- **WHEN** a SignalSource is applied without `spec.type`, or an existing one's `spec.type` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Wiring fields no longer accepted
- **WHEN** a manifest sets `spec.channelRef` or `spec.profileRef` against the new CRD
- **THEN** the fields are pruned/rejected, and the documented migration (a Pipeline claiming the source) provides the routing

#### Scenario: Credentials declared on the source, materialized only by the kubelet
- **WHEN** a SignalSource sets `credentialsSecretRef: {name: pd-api-key}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API
