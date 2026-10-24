# alertmanager-signal-adapter

## Purpose

The webhook-receiving Alertmanager signal adapter in `signals/alertmanager/`: a dependency-free module that serves SignalSources through the signal adapter contract, accepting Alertmanager-format webhooks and normalizing firing alerts carrying the sender's own group identity (`initiationId`, labels, titles) while cooldown and routing stay manager-side, keyed on that id. Webhook auth is opt-in via source credentials with constant-time bearer-token comparison.

## Requirements

### Requirement: Alertmanager runs as a webhook-receiving signal adapter
A signal adapter in `signals/alertmanager/` (own dependency-free Go module and image, precedents `signals/cron/` and `channels/telegram/`) SHALL serve SignalSources whose `spec.adapter` names its SignalAdapter CR, through the signal adapter contract, while exposing its own HTTP server accepting Alertmanager-format webhooks at `POST /webhook/{source}` — **listening on the port declared by `SignalAdapter.spec.port` (the reconciler injects `LISTEN_ADDR` and owns the Service); hand-deployed instances set `LISTEN_ADDR`/`ADAPTER_NAME` themselves**. The `{source}` path segment SHALL route to a served SignalSource learned from `GET /signal/sources?adapter=<key>`; posts for unknown or unserved sources SHALL return 404. The payload format is the standard Alertmanager webhook format — this adapter IS the operator's Alertmanager ingestion path; the manager hosts none. For discoverability, the adapter's Ready report for each served source SHALL name the webhook path, and the chart's install notes SHALL print the full Service URL per rendered source.

#### Scenario: Unknown source is refused
- **WHEN** a webhook is posted for a source name that no served SignalSource matches
- **THEN** the adapter returns 404 naming the source and pushes nothing, and the sender's retry succeeds once the source is served

#### Scenario: Alertmanager-format alerts become conversations
- **WHEN** a SignalSource typed with the adapter's key is served and a sender posts a firing alert to `/webhook/<source>`
- **THEN** a normalized signal is pushed to `/signal/inbound` and the manager's ingest core creates or reuses a conversation keyed on its `initiationId`

#### Scenario: Webhook endpoint is discoverable from source status
- **WHEN** the adapter reports a served source Ready
- **THEN** the source's Ready condition message names the `/webhook/<source>` path an operator should target

### Requirement: Alerts normalize carrying the sender's group identity
The adapter SHALL drop non-`firing` alerts and, per firing alert, emit one signal with:

- `initiationId`: the webhook's `groupKey` verbatim — Alertmanager's `group_by` is the grouping decision, made upstream, so every alert of one group correlates to one conversation. When the body carries no `groupKey`, a deterministic fallback derived by hashing the sorted `groupLabels` pairs (an empty id is rejected by `/signal/inbound`)
- the raw alert label map as `labels`
- title `"🔍 " + alertname` plus ` — <namespace>` when that label is present
- a per-alert JSON payload carrying labels, annotations, `startsAt`, and `generatorURL`
- no `kind` (alert lane)

The adapter SHALL NOT deduplicate, apply cooldown, or route. Which alerts are one problem is the sender's `group_by` decision, carried in the id — the dedup window stays manager-side cooldown on that id, and routing stays manager-side id equality.

#### Scenario: Only firing alerts produce signals
- **WHEN** a webhook body contains two `firing` and one `resolved` alert
- **THEN** exactly two signals are pushed and the response reports what was queued

#### Scenario: One group is one conversation
- **WHEN** two firing alerts of one Alertmanager group arrive, in one webhook or across two
- **THEN** every signal carries that group's `groupKey` as its `initiationId` and they land in one conversation

#### Scenario: Regrouping upstream re-routes
- **WHEN** `group_by` changes upstream so an alert arrives under a `groupKey` no open conversation carries
- **THEN** a new conversation is created adopting that id

#### Scenario: Missing groupKey gets a stable fallback
- **WHEN** a nonstandard sender posts twice without a `groupKey` and with identical `groupLabels`
- **THEN** both posts carry the same derived `initiationId` and the manager's cooldown collapses the repeat

### Requirement: Webhook auth is opt-in via source credentials
When a served source's listing entry advertises a `credentialEnvPrefix` (the SignalSource set `credentialsSecretRef`), the adapter SHALL require `Authorization: Bearer <token>` on `/webhook/{source}` matching the projected `AGENTOPS_CRED_<SOURCE>_TOKEN` value, compared in constant time, returning 401 on mismatch or absence. Sources without credentials SHALL accept unauthenticated posts. The manager SHALL continue to read no Secrets — the token reaches the adapter only through the existing kubelet-resolved credential projection.

#### Scenario: Credentialed source rejects anonymous posts
- **WHEN** a source has `credentialsSecretRef` with a `TOKEN` key and a webhook arrives without the matching bearer token
- **THEN** the adapter returns 401 and pushes nothing

#### Scenario: Uncredentialed source stays open
- **WHEN** a source has no `credentialsSecretRef`
- **THEN** unauthenticated webhooks for it are accepted, matching the built-in endpoint's posture

### Requirement: The adapter self-registers with the sender from source config

For each served SignalSource whose opaque config carries a `register` block,
the adapter SHALL ensure a `VMAlertmanagerConfig` named `agentops-<source>`
in the block's target namespace.

The object SHALL contain a webhook receiver pointing at the adapter's own
endpoint
(`http://agentops-signal-<adapter>.<POD_NAMESPACE>.svc:<port>/webhook/<source>`)
and a route with `continue: true`, never diverting alerts from existing
receivers.

The block's fields, every one optional:

| Field | Means |
|---|---|
| `namespace` | the namespace to register in — absent, the adapter's own namespace |
| `matchers`, `groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts`, `sendResolved` | route and receiver knobs, together enough to fully REPLACE a hand-written receiver plus route |

There is no `name` field. The registered object's name is always the derived
`agentops-<source>`. Unset knobs SHALL be omitted from the rendered object
rather than written as zero values, so the sender's own default applies.

The ensure SHALL use the in-cluster Kubernetes REST API with the mounted
ServiceAccount token, implemented with the standard library only — the module
stays dependency-free.

It SHALL run on every registry poll: created when absent, updated on drift.
Sources without `register` SHALL behave exactly as before. On success the
source's Ready condition SHALL report `reason: AdapterReady` naming the
registered object.

#### Scenario: Registration from source config
- **WHEN** a served source's config carries `register: {namespace: monitoring}` and the adapter pod has a token with write access to vmalertmanagerconfigs in `monitoring`
- **THEN** a `VMAlertmanagerConfig` `monitoring/agentops-<source>` exists with the adapter's webhook URL and a continue-route, and the source's Ready message names it

#### Scenario: An empty register block registers beside the adapter
- **WHEN** a served source's config carries `register: {}` with no `namespace`
- **THEN** the object is ensured in the adapter's own namespace, under the same derived name

#### Scenario: Drift is repaired
- **WHEN** the registered object is edited to point elsewhere
- **THEN** a subsequent poll restores the adapter's webhook URL

#### Scenario: Source-owned routing replaces a hand-written receiver
- **WHEN** a source's `register` block carries matchers plus `groupWait`, `repeatInterval` and `maxAlerts`
- **THEN** the rendered route carries those matchers and timings and the webhook carries `max_alerts`, so the equivalent hand-written receiver and route can be deleted from the sender's own config

#### Scenario: An unset knob is omitted, not zeroed
- **WHEN** a source's `register` block leaves `sendResolved` unset
- **THEN** the rendered webhook carries no `send_resolved` key at all, so the sender's own default governs

#### Scenario: No register block, no registration
- **WHEN** a served source's config carries no `register`
- **THEN** the adapter performs no Kubernetes API calls for it and the Ready message names the webhook path as before

### Requirement: Registration failure degrades to instructions, never to an unserved source

When registration cannot be performed, the adapter SHALL keep serving the
source (the webhook stays live) and SHALL report the Ready condition True
with `reason: RegistrationManual`.

The message SHALL state the cause plus the exact manual action: the full
webhook URL and the object to create (`VMAlertmanagerConfig
<ns>/agentops-<source>` or an equivalent `webhook_configs` entry).

The degradation cases, each loud and none silent:

| Cause | The message additionally states |
|---|---|
| token not mounted, or the API forbidden (RBAC missing) | the grant to bind — against the account `spec.serviceAccountName` names |
| `VMAlertmanagerConfig` CRD absent, or target unreachable | the manual `webhook_configs` form |
| a malformed `register` block | what failed to parse — never treated as an absent block |
| the adapter cannot determine its own webhook URL (hand-deployed with no namespace fact) | the URL shape to fill in — never a placeholder written into the sender's live config |

A placeholder URL in a registered object looks registered and delivers
nothing, so computing no URL SHALL mean registering no object.

The message SHALL name only fields the current CRD carries, so following it
can actually heal the condition.

The attempt SHALL be retried on every registry poll, so externally granting
the missing permission or correcting the block heals the condition without a
restart.

#### Scenario: Missing RBAC yields instructions
- **WHEN** the adapter's ensure call returns 403
- **THEN** the source stays served and its Ready message contains the webhook URL and the manual registration step naming the target namespace

#### Scenario: A malformed register block is named, not ignored
- **WHEN** a served source's `register` block cannot be parsed
- **THEN** the source stays served, Ready reports `RegistrationManual` naming the parse problem, and no registration call is attempted

#### Scenario: An uncomputable webhook URL registers nothing
- **WHEN** the adapter holds a token but cannot determine its own namespace, so it cannot compute its webhook URL
- **THEN** no object is written, and Ready reports `RegistrationManual` with the URL shape the operator must fill in

#### Scenario: The remediation can be followed
- **WHEN** registration fails for a missing grant
- **THEN** the message names `spec.serviceAccountName` and the binding to create, and names no field the CRD does not carry

#### Scenario: Granting permission self-heals
- **WHEN** the operator later binds the required Role to the adapter's ServiceAccount
- **THEN** a subsequent poll registers successfully and the Ready message flips to the registered-object form
