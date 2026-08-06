# signal-source-model

## Purpose

The restructured `SignalSource` CRD: an open string `spec.type` with opaque per-type `config` and name-only credential references, manager-side grouping policy applied uniformly to every source type, and unchanged in-process compatibility for the built-in `alertmanagerWebhook` type.

## Requirements

### Requirement: SignalSource CRD splits shared metadata from type-specific config
The `SignalSource` CRD SHALL consist of type-agnostic metadata — required immutable open-string `spec.type`, `spec.profileRef`, optional `spec.channelRef`, typed `spec.grouping` (signatureLabels, windowDays, cooldownHours), optional `spec.credentialsSecretRef` — plus an optional opaque `spec.config` (`x-kubernetes-preserve-unknown-fields`) whose shape only the serving signal implementation defines and validates. The operator SHALL never interpret `spec.config` and SHALL never read the credential Secret's values (name-only projection). The `cron` and `events` typed sub-structs SHALL be removed (**BREAKING** schema-only; neither was ever implemented).

#### Scenario: Arbitrary config accepted for any type
- **WHEN** a SignalSource is applied with `type: pagerduty` and a `config` object the operator has never seen
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation

#### Scenario: Type is required and immutable
- **WHEN** a SignalSource is applied without `spec.type`, or an existing one's `spec.type` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Existing alertmanager sources survive the upgrade untouched
- **WHEN** the upgraded CRD is applied to a cluster with a `type: alertmanagerWebhook` SignalSource
- **THEN** the CR remains valid without any edit and its ingest behavior is unchanged

#### Scenario: Credentials declared on the source, materialized only by the kubelet
- **WHEN** a SignalSource sets `credentialsSecretRef: {name: pd-api-key}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

### Requirement: Grouping policy stays manager-side for every source type
Signature grouping, fingerprint cooldown, window-based conversation reuse, and recurrence-on-session SHALL be applied by the manager from `spec.grouping` for signals of every source type — built-in or adapter-fed. Adapters SHALL NOT need to implement any grouping logic.

#### Scenario: Adapter-fed signals group like built-in ones
- **WHEN** two normalized signals with different fingerprints but identical signature labels arrive via an adapter within the source's window
- **THEN** they land in the same conversation, the second as a recurrence when a session exists

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

### Requirement: Built-in Alertmanager webhook remains in-process and unchanged
`type: alertmanagerWebhook` SHALL remain served in-process by the manager at `POST /ingest/alertmanager/{source}` with unchanged external behavior (payload format, firing filter, grouping, status bookkeeping), internally routed through the same normalized-signal core the adapter contract uses.

#### Scenario: Existing Alertmanager configuration keeps working
- **WHEN** Alertmanager posts to the pre-existing webhook URL after the upgrade
- **THEN** alerts group into conversations exactly as before, with no Alertmanager or CR reconfiguration
