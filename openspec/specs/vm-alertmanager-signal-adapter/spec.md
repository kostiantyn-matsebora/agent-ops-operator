# vm-alertmanager-signal-adapter

## Purpose

The webhook-receiving VM Alertmanager signal adapter in `signal-vmalertmanager/`: a dependency-free module that serves `type: vmAlertmanagerWebhook` SignalSources through the signal adapter contract, accepting Alertmanager-format webhooks and normalizing firing alerts with built-in-path parity (fingerprints, labels, titles) while grouping stays manager-side. Webhook auth is opt-in via source credentials with constant-time bearer-token comparison.

## Requirements

### Requirement: VM Alertmanager runs as a webhook-receiving signal adapter
A signal adapter in `signal-vmalertmanager/` (own dependency-free Go module and image, precedents `signal-cron/` and `channel-telegram/`) SHALL serve SignalSources whose `spec.adapter` names its SignalAdapter CR, through the signal adapter contract, while exposing its own HTTP server accepting Alertmanager-format webhooks at `POST /webhook/{source}` — **listening on the port declared by `SignalAdapter.spec.port` (the reconciler injects `LISTEN_ADDR` and owns the Service); hand-deployed instances set `LISTEN_ADDR`/`ADAPTER_NAME` themselves**. The `{source}` path segment SHALL route to a served SignalSource learned from `GET /signal/sources?adapter=<key>`; posts for unknown or unserved sources SHALL return 404. The payload format is the standard Alertmanager webhook format — this adapter IS the operator's Alertmanager ingestion path; the manager hosts none. For discoverability, the adapter's Ready report for each served source SHALL name the webhook path, and the chart's install notes SHALL print the full Service URL per rendered source.

#### Scenario: Unknown source is refused
- **WHEN** a webhook is posted for a source name that no served SignalSource matches
- **THEN** the adapter returns 404 naming the source and pushes nothing, and the sender's retry succeeds once the source is served

#### Scenario: Alertmanager-format alerts become conversations
- **WHEN** a SignalSource typed with the adapter's key is served and a sender posts a firing alert to `/webhook/<source>`
- **THEN** a normalized signal is pushed to `/signal/inbound` and the manager's ingest core creates or reuses a conversation per its grouping config

#### Scenario: Webhook endpoint is discoverable from source status
- **WHEN** the adapter reports a served source Ready
- **THEN** the source's Ready condition message names the `/webhook/<source>` path an operator should target

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

### Requirement: The adapter self-registers with the sender from source config
For each served SignalSource whose opaque config carries a `register` block
(`register.vmalertmanager: {name, namespace}` required; `matchers`,
`groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts` and
`sendResolved` optional — together enough to express everything a
hand-written receiver + route did, so a SignalSource can fully REPLACE one
rather than sit alongside it; unset knobs are omitted from the rendered
object rather than written as zero values), the adapter SHALL ensure a `VMAlertmanagerConfig`
named `agentops-<source>` in the target namespace containing a webhook
receiver pointing at its own endpoint
(`http://agentops-signal-<adapter>.<POD_NAMESPACE>.svc:<port>/webhook/<source>`)
and a route with `continue: true` (never diverting alerts from existing
receivers), using the in-cluster Kubernetes REST API with the mounted
ServiceAccount token — implemented with the standard library only (the
module stays dependency-free). The ensure SHALL run on every registry poll:
created when absent, updated on drift. Sources without `register` SHALL
behave exactly as before. On success the source's Ready condition SHALL
report `reason: AdapterReady` naming the registered object.

#### Scenario: Registration from source config
- **WHEN** a served source's config carries `register.vmalertmanager: {name: vmks, namespace: monitoring}` and the adapter pod has a token with write access to vmalertmanagerconfigs in `monitoring`
- **THEN** a `VMAlertmanagerConfig` `monitoring/agentops-<source>` exists with the adapter's webhook URL and a continue-route, and the source's Ready message names it

#### Scenario: Drift is repaired
- **WHEN** the registered object is edited to point elsewhere
- **THEN** a subsequent poll restores the adapter's webhook URL

#### Scenario: Source-owned routing replaces a hand-written receiver
- **WHEN** a source's `register` block carries matchers plus `groupWait`, `repeatInterval` and `maxAlerts`
- **THEN** the rendered route carries those matchers and timings and the webhook carries `max_alerts`, so the equivalent hand-written receiver and route can be deleted from the sender's own config

#### Scenario: No register block, no registration
- **WHEN** a served source's config carries no `register`
- **THEN** the adapter performs no Kubernetes API calls for it and the Ready message names the webhook path as before

### Requirement: Registration failure degrades to instructions, never to an unserved source
When registration cannot be performed — token not mounted, API forbidden
(RBAC missing), `VMAlertmanagerConfig` CRD absent, or target unreachable —
the adapter SHALL keep serving the source (webhook stays live) and SHALL
report the Ready condition True with `reason: RegistrationManual` and a
message stating the cause plus the exact manual action: the full webhook
URL and the object to create (`VMAlertmanagerConfig <ns>/agentops-<source>`
or an equivalent `webhook_configs` entry). The attempt SHALL be retried on
every registry poll so externally granting the missing permission heals the
condition without a restart.

#### Scenario: Missing RBAC yields instructions
- **WHEN** the adapter's ensure call returns 403
- **THEN** the source stays served and its Ready message contains the webhook URL and the manual registration step naming the target namespace

#### Scenario: Granting permission self-heals
- **WHEN** the operator later binds the required Role to the adapter's ServiceAccount
- **THEN** a subsequent poll registers successfully and the Ready message flips to the registered-object form
