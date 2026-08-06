# channel-type-model — delta

## MODIFIED Requirements

### Requirement: Channel CRD splits shared metadata from type-specific config
The `Channel` CRD SHALL consist of type-agnostic metadata — required immutable `spec.type` (string channel-type identifier), optional `spec.defaultProfileRef`, optional `spec.delivery` hints, optional `spec.credentialsSecretRef` (LocalObjectReference naming the Secret holding this surface's transport credentials — credentials are per-surface usage, never per-implementation) — plus an optional opaque `spec.config` object (`x-kubernetes-preserve-unknown-fields`) whose shape is defined and validated solely by the channel-type implementation. The operator SHALL never interpret `spec.config` and SHALL never read the referenced credential Secret's values (it only projects the secret *name* into adapter pod specs). The typed `spec.telegram` sub-struct SHALL be removed (**BREAKING**; migration documented).

#### Scenario: Arbitrary config accepted for any type
- **WHEN** a Channel is applied with `type: slack` and a `config` object with fields the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Type is required and immutable
- **WHEN** a Channel is applied without `spec.type`, or an existing Channel's `spec.type` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Old shape no longer served
- **WHEN** a manifest using the removed `spec.telegram` sub-struct is applied against the new CRD
- **THEN** validation fails, and the documented migration (telegram sub-struct → `type: telegram` + `config`) produces an accepted equivalent

#### Scenario: Credentials declared on the surface, materialized only by the kubelet
- **WHEN** a Channel sets `credentialsSecretRef: {name: ops-bot}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API
