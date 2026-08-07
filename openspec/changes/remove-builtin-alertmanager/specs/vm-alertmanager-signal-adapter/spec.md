# vm-alertmanager-signal-adapter — delta

## MODIFIED Requirements

### Requirement: VM Alertmanager runs as a webhook-receiving signal adapter
A signal adapter in `signal-vmalertmanager/` (own dependency-free Go module and image, precedents `signal-cron/` and `channel-telegram/`) SHALL serve SignalSources with `spec.type: vmAlertmanagerWebhook` through the signal adapter contract while exposing its own HTTP server (default `:8080`, overridable via `LISTEN_ADDR` env) accepting Alertmanager-format webhooks at `POST /webhook/{source}`. The `{source}` path segment SHALL route to a served SignalSource learned from `GET /signal/sources?type=vmAlertmanagerWebhook`; posts for unknown or unserved sources SHALL return 404. The payload format is the standard Alertmanager webhook format (any Alertmanager-compatible sender works) — **this adapter IS the operator's Alertmanager ingestion path; the manager hosts none**. For discoverability, the adapter's Ready report for each served source SHALL name the webhook path, and the chart's install notes SHALL print the full Service URL per rendered source.

#### Scenario: Alertmanager-format alerts become conversations
- **WHEN** a `type: vmAlertmanagerWebhook` SignalSource is served and a sender posts a firing alert to `/webhook/<source>`
- **THEN** a normalized signal is pushed to `/signal/inbound` and the manager's ingest core creates or reuses a conversation per its grouping config

#### Scenario: Unknown source is refused
- **WHEN** a webhook is posted for a source name that no served SignalSource matches
- **THEN** the adapter returns 404 naming the source and pushes nothing, and the sender's retry succeeds once the source is served

#### Scenario: Webhook endpoint is discoverable from source status
- **WHEN** the adapter reports a served source Ready
- **THEN** the source's Ready condition message names the `/webhook/<source>` path an operator should target
