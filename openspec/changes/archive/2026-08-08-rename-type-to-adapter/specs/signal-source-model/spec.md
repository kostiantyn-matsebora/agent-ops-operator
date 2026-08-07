# signal-source-model — delta

## MODIFIED Requirements

### Requirement: SignalSource CRD splits shared metadata from type-specific config
The `SignalSource` CRD SHALL consist of type-agnostic metadata — **required immutable `spec.adapter`, naming the `SignalAdapter` whose implementation serves this source and whose schema governs `spec.config`** — plus typed `spec.grouping` (signatureLabels, windowDays, cooldownHours) and optional `spec.credentialsSecretRef`, and an optional opaque `spec.config` (`x-kubernetes-preserve-unknown-fields`) whose shape only that adapter defines and validates. As on `Channel`, selector and config stay siblings and the selector is named for what it references. The operator SHALL never interpret `spec.config` and SHALL never read the credential Secret's values (name-only projection). The source SHALL carry no wiring — a `Pipeline` claim is the only way a source reaches a profile and channels.

#### Scenario: Arbitrary config accepted for any adapter
- **WHEN** a SignalSource is applied with `adapter: pagerduty` and a `config` object the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Adapter reference is required and immutable
- **WHEN** a SignalSource is applied without `spec.adapter`, or an existing one's `spec.adapter` is changed
- **THEN** the API server rejects the request with a validation error
