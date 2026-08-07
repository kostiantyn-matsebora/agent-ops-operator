# Proposal: remove-builtin-alertmanager

## Why

With the `signal-vmalertmanager` adapter live, the manager's built-in `alertmanagerWebhook` type is a duplicate implementation of the same webhook format — two similar SignalSources on a live cluster, two code paths, one of them special-cased everywhere (Served condition, docs, quickstart). "Adapters normalize, the manager groups" taken to its conclusion: the manager should host no signal transport at all (user decision).

## What Changes

- **BREAKING** — Remove the built-in `alertmanagerWebhook` signal type from the manager: the `POST /ingest/alertmanager/{source}` endpoint, the Alertmanager payload parsing in `httpapi`, the `SourceAlertmanager` constant, and the built-in special case in the SignalSource `Served` condition (every signal type now needs a serving adapter).
- The normalized-signal core, grouping, cooldown, recurrence, and the `/signal/*` contract are untouched — only the manager-hosted transport goes.
- Migration is a webhook repoint: senders move to the `signal-vmalertmanager` adapter (`/webhook/{source}` behind its Service), sources move to `type: vmAlertmanagerWebhook`. The live install repoints VMAlertmanager BEFORE this deploys (no alert-loss window); the old `alertmanager` source and its pipeline claim retire after.
- Docs/samples: README quickstart + HTTP API table + migration note; samples lose the built-in source (the vm-bundle samples already show the replacement).

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `signal-source-model`: the "Built-in Alertmanager webhook remains in-process and unchanged" requirement is REMOVED; grouping-parity language stops referencing a built-in path.
- `signal-adapter-lifecycle`: `Served` no longer has a built-in exception — a Ready SignalAdapter (or adapter-reported readiness) is the only way a source is served.
- `signal-adapter-contract`: wording drops "same core as the built-in webhook" (the core is simply THE ingest core).
- `vm-alertmanager-signal-adapter`: the "built-in path unaffected" language is replaced — this adapter IS the Alertmanager path.

## Impact

- `internal/httpapi/server.go` (route, handler, AM types deleted), `internal/controller/signalsource_controller.go` (built-in case), `api/v1alpha1/signalsource_types.go` (constant), integration tests (AM grouping test re-expressed via `/signal/inbound`), samples, README, CLAUDE.md.
- Chart 1.6.0 / manager 0.8.0. Live deploy gated on the operator repointing VMAlertmanager (their GitOps repo) — code ships first, upgrade after confirmation.
