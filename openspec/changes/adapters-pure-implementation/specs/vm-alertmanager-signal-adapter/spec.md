# vm-alertmanager-signal-adapter — delta

## MODIFIED Requirements

### Requirement: VM Alertmanager runs as a webhook-receiving signal adapter
A signal adapter in `signal-vmalertmanager/` (own dependency-free Go module and image, precedents `signal-cron/` and `channel-telegram/`) SHALL serve SignalSources whose `spec.type` names its SignalAdapter CR, through the signal adapter contract, while exposing its own HTTP server accepting Alertmanager-format webhooks at `POST /webhook/{source}` — **listening on the port declared by `SignalAdapter.spec.port` (the reconciler injects `LISTEN_ADDR` and owns the Service); hand-deployed instances set `LISTEN_ADDR`/`SOURCE_TYPE` themselves**. The `{source}` path segment SHALL route to a served SignalSource learned from `GET /signal/sources?type=<key>`; posts for unknown or unserved sources SHALL return 404. The payload format is the standard Alertmanager webhook format — this adapter IS the operator's Alertmanager ingestion path; the manager hosts none. For discoverability, the adapter's Ready report for each served source SHALL name the webhook path, and the chart's install notes SHALL print the full Service URL per rendered source.

#### Scenario: Alertmanager-format alerts become conversations
- **WHEN** a SignalSource typed with the adapter's key is served and a sender posts a firing alert to `/webhook/<source>`
- **THEN** a normalized signal is pushed to `/signal/inbound` and the manager's ingest core creates or reuses a conversation per its grouping config

#### Scenario: Unknown source is refused
- **WHEN** a webhook is posted for a source name that no served SignalSource matches
- **THEN** the adapter returns 404 naming the source and pushes nothing, and the sender's retry succeeds once the source is served

#### Scenario: Webhook endpoint is discoverable from source status
- **WHEN** the adapter reports a served source Ready
- **THEN** the source's Ready condition message names the `/webhook/<source>` path an operator should target
