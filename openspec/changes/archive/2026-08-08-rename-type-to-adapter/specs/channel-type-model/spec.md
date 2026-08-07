# channel-type-model — delta

## MODIFIED Requirements

### Requirement: Channel CRD splits shared metadata from type-specific config
The `Channel` CRD SHALL consist of type-agnostic metadata — **required immutable `spec.adapter`, naming the `ChannelAdapter` whose implementation serves this surface and whose schema governs `spec.config`** — plus optional `spec.credentialsSecretRef` (LocalObjectReference naming the Secret holding this surface's transport credentials, per-surface usage, never per-implementation) and an optional opaque `spec.config` object (`x-kubernetes-preserve-unknown-fields`) whose shape is defined and validated solely by that adapter. `spec.adapter` and `spec.config` stay siblings, matching how Kubernetes pairs a selector with its implementation-owned config (`StorageClass.provisioner`/`parameters`, `IngressClass.controller`/`parameters`); the name says the value is a REFERENCE, so `config` is never mistaken for part of one flat schema the selector belongs to. The operator SHALL never interpret `spec.config` and SHALL never read the referenced credential Secret's values (it only projects the secret *name* into adapter pod specs). The channel SHALL carry no wiring and no delivery mode — a channel's default profile for bare messages comes from its oldest Ready `Pipeline`, and the operator delivers all agent output.

#### Scenario: Arbitrary config accepted for any adapter
- **WHEN** a Channel is applied with `adapter: slack` and a `config` object with fields the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Adapter reference is required and immutable
- **WHEN** a Channel is applied without `spec.adapter`, or an existing Channel's `spec.adapter` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: The former type field is gone
- **WHEN** a Channel is applied carrying `spec.type`
- **THEN** the field is not part of the schema and is pruned rather than honoured, so a stale manifest cannot silently select an adapter
