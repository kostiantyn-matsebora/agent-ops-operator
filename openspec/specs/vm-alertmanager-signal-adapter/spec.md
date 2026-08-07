# vm-alertmanager-signal-adapter

## Purpose

The webhook-receiving VM Alertmanager signal adapter in `signal-vmalertmanager/`: a dependency-free module that serves `type: vmAlertmanagerWebhook` SignalSources through the signal adapter contract, accepting Alertmanager-format webhooks and normalizing firing alerts with built-in-path parity (fingerprints, labels, titles) while grouping stays manager-side. Webhook auth is opt-in via source credentials with constant-time bearer-token comparison.

## Requirements

### Requirement: VM Alertmanager runs as a webhook-receiving signal adapter
A signal adapter in `signal-vmalertmanager/` (own dependency-free Go module and image, precedents `signal-cron/` and `channel-telegram/`) SHALL serve SignalSources with `spec.type: vmAlertmanagerWebhook` through the signal adapter contract while exposing its own HTTP server (default `:8080`, overridable via `LISTEN_ADDR` env) accepting Alertmanager-format webhooks at `POST /webhook/{source}`. The `{source}` path segment SHALL route to a served SignalSource learned from `GET /signal/sources?type=vmAlertmanagerWebhook`; posts for unknown or unserved sources SHALL return 404. The payload format is the standard Alertmanager webhook format (any Alertmanager-compatible sender works; VictoriaMetrics VMAlertmanager is the packaged story). A `SourceVMAlertmanager = "vmAlertmanagerWebhook"` constant SHALL name the type in `api/v1alpha1`; the built-in in-process `alertmanagerWebhook` type and its endpoint SHALL remain untouched.

#### Scenario: VMAlertmanager alerts become conversations
- **WHEN** a `type: vmAlertmanagerWebhook` SignalSource is served and VMAlertmanager posts a firing alert to `/webhook/<source>`
- **THEN** a normalized signal is pushed to `/signal/inbound` and the manager's ingest pipeline creates or reuses a conversation per its grouping config

#### Scenario: Unknown source is refused
- **WHEN** a webhook is posted for a source name that no served SignalSource matches
- **THEN** the adapter returns 404 naming the source and pushes nothing, and the sender's retry succeeds once the source is served

#### Scenario: Built-in path is unaffected
- **WHEN** the adapter is deployed alongside existing `type: alertmanagerWebhook` sources
- **THEN** posts to the manager's `/ingest/alertmanager/{source}` behave exactly as before

### Requirement: Alerts normalize with built-in-path parity
The adapter SHALL drop non-`firing` alerts and, per firing alert, emit one signal with: the Alertmanager `fingerprint` verbatim, or — when absent — a deterministic fallback derived by hashing the sorted label pairs (empty fingerprints are rejected by `/signal/inbound`); the raw alert label map as `labels`; title `"🔍 " + alertname` plus ` — <namespace>` when that label is present; a per-alert JSON payload carrying labels, annotations, `startsAt`, and `generatorURL`; and no `kind` (alert lane). The adapter SHALL NOT group, deduplicate, or apply cooldown — signature grouping, fingerprint cooldown, window reuse, and recurrence stay manager-side from `SignalSource.spec.grouping`.

#### Scenario: Only firing alerts produce signals
- **WHEN** a webhook body contains two `firing` and one `resolved` alert
- **THEN** exactly two signals are pushed and the response reports what was queued

#### Scenario: Missing fingerprint gets a stable fallback
- **WHEN** an alert arrives without a `fingerprint` field twice with identical labels
- **THEN** both posts carry the same derived fingerprint and the manager's cooldown collapses the repeat

#### Scenario: Grouping matches the built-in path
- **WHEN** two firing alerts share values for the source's `signatureLabels`
- **THEN** they land in the same conversation, exactly as the same alerts would via the built-in endpoint

### Requirement: Webhook auth is opt-in via source credentials
When a served source's listing entry advertises a `credentialEnvPrefix` (the SignalSource set `credentialsSecretRef`), the adapter SHALL require `Authorization: Bearer <token>` on `/webhook/{source}` matching the projected `AGENTOPS_CRED_<SOURCE>_TOKEN` value, compared in constant time, returning 401 on mismatch or absence. Sources without credentials SHALL accept unauthenticated posts. The manager SHALL continue to read no Secrets — the token reaches the adapter only through the existing kubelet-resolved credential projection.

#### Scenario: Credentialed source rejects anonymous posts
- **WHEN** a source has `credentialsSecretRef` with a `TOKEN` key and a webhook arrives without the matching bearer token
- **THEN** the adapter returns 401 and pushes nothing

#### Scenario: Uncredentialed source stays open
- **WHEN** a source has no `credentialsSecretRef`
- **THEN** unauthenticated webhooks for it are accepted, matching the built-in endpoint's posture
