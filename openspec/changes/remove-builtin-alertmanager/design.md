# remove-builtin-alertmanager — design

## Context

The manager still hosts one signal transport itself: `POST /ingest/alertmanager/{source}` parsing Alertmanager payloads into the normalized core. Everything else already flows through adapters, and the `signal-vmalertmanager` adapter (live, verified) accepts the identical webhook format. The built-in is special-cased in the Served condition, the docs, and the live install (two near-identical sources). User decision: remove it.

## Goals / Non-Goals

**Goals:** one Alertmanager path (the adapter); manager hosts zero signal transports; no alert-loss during the live cutover.
**Non-Goals:** no changes to the ingest core (grouping/cooldown/recurrence/`routeSignals`), the `/signal/*` contract, or the adapter; no removal of the `/ingest` concept from history/migration docs.

## Decisions

- **D1: Transport-only removal** — delete the route, `handleAlertmanager`, the `amAlert`/`amPayload` types, and the `SourceAlertmanager` constant. `routeSignals`, `combineFunc`, cooldowns, and `/signal/inbound` stay exactly as they are (the AM-specific combine closure dies with the handler; `combineJoined` remains the generic one).
- **D2: Served is adapter-only** — `SignalSourceReconciler` drops the built-in branch: every source type needs a Ready `SignalAdapter` (or adapter-reported readiness). No registry concept for signals at all.
- **D3: Test parity moves to the contract** — the AM-specific grouping test is re-expressed via `/signal/inbound` (multi-signal same-signature batch → ONE conversation input, the previously AM-only combine path); recurrence/cooldown/grouping remain covered by existing signal tests. The vmalertmanager module's unit tests already own AM payload parsing.
- **D4: Live cutover order is the safety property** — (1) operator repoints VMAlertmanager (GitOps repo) to the adapter Service and confirms alerts arrive via `vm-alerts`; (2) manager 0.8.0 / chart 1.6.0 deploys (endpoint gone); (3) old `alertmanager` source + `home-ops-pipeline` claim retire. Code merges before (1); the upgrade waits for it.

## Risks / Trade-offs

- [Deploying before the repoint drops alerts] → hard gate: live upgrade only after the operator confirms the repoint (VMAlertmanager retries + manager cooldown make the switchover itself lossless).
- [Other users of the built-in endpoint] → pre-1.0 policy; README migration section gives the adapter equivalent (vm-bundle or a plain SignalAdapter CR).
- [Quickstart loses a zero-install ingest demo] → the quickstart's actual demo is `/task` (unchanged); alert ingest was always a configure-your-Alertmanager step and now says to enable the bundle.

## Migration Plan

Per D4. Rollback: previous chart/manager restores the endpoint; sources are untouched by this change (only the constant, code, and docs move).

## Open Questions

- None.
