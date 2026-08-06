# Proposal: make-signal-source-as-well-extendable-as-channel

## Why

Channel types are now fully pluggable (open `type` + opaque `config`, HTTP contract, `ChannelAdapter` CR) — but signal sources are still compiled in: `SignalSource.spec.type` is a closed enum whose only implemented member is `alertmanagerWebhook`, its parsing lives in the manager, and the `cron`/`k8sEvents` roadmap placeholders are typed sub-structs that would each require an operator release. Adopters should plug in new signal kinds (PagerDuty, email, cron, k8s events, custom webhooks) exactly like channel types: an image + a CR, zero operator changes.

## What Changes

- Restructure `SignalSource` the way `Channel` was restructured: `spec.type` becomes an **open string**, type-specific settings move to an opaque **`spec.config`** (`x-kubernetes-preserve-unknown-fields`), and `spec.credentialsSecretRef` declares per-source transport credentials (projected, never read). Type-agnostic metadata stays typed: `channelRef`, `profileRef`, `grouping` — grouping/cooldown/recurrence remain the manager's value-add for every source type. Existing `alertmanagerWebhook` CRs stay valid unchanged (enum relaxed, not renamed).
- **BREAKING (schema only)** — remove the never-implemented `spec.cron` and `spec.events` sub-structs; their future implementations are adapters with their shape in `config`. No live CR can be using them (the types were never wired).
- Define a **signal adapter contract** — the inbound-only sibling of the channel contract (no ops queue): push normalized signals (`fingerprint`, `labels`, optional `title`, payload, alert-vs-job kind) into the manager, which applies cooldown, signature grouping, window reuse, and recurrence through the existing ingest core; plus source listing (opaque config + `credentialEnvPrefix`), cursor state, and Ready-condition reporting.
- New **`SignalAdapter` CRD + reconciler** mirroring `ChannelAdapter`: pure implementation (`type` + `image`, never credentials), owned Deployment with zero-RBAC SA, singleton discipline, per-adapter derived token (distinct derivation context so a ChannelAdapter and SignalAdapter sharing a name never share a token), credential projection from served SignalSources, `TypeConflict` guard, and a `Served` condition on SignalSources.
- The built-in Alertmanager webhook stays in-process (it needs the manager's HTTP surface, like the web channel): `POST /ingest/alertmanager/{source}` is unchanged externally, refactored internally to feed the same normalized-signal routing core the contract uses.
- Ship a reference **cron signal adapter** (`signal-cron/`, own module, dependency-free — precedent `channel-telegram/`): fires scheduled job-lane inputs from `config: {schedule, input}`, cursor via the state API — replacing the removed `CronSpec` with the first proof of the contract.
- Chart: `signaladapters` CRD; no new default workloads.

## Capabilities

### New Capabilities

- `signal-source-model`: the restructured SignalSource CRD — open `type`, opaque `config`, `credentialsSecretRef`, typed grouping/routing metadata, removal of `cron`/`events` sub-structs, built-in `alertmanagerWebhook` compatibility.
- `signal-adapter-contract`: the manager↔signal-adapter HTTP API — normalized inbound signals through the shared grouping core, source listing with credential prefix, cursor state, status reporting, and type-scoped derived-token auth.
- `signal-adapter-lifecycle`: the `SignalAdapter` CRD and reconciler — workload ownership, credential projection, type-conflict guard, Served visibility.
- `cron-signal-adapter`: the reference adapter — schedule parsing, job-lane signals, restart-safe cursor.

### Modified Capabilities

<!-- none: channel-* specs remain true as written; signal auth defines its own
     derivation context in signal-adapter-contract rather than modifying
     channel-adapter-contract -->

## Impact

- `api/v1alpha1/`: `signalsource_types.go` restructured (+ new `signaladapter_types.go`); regenerated deepcopy/CRDs; samples updated.
- `internal/httpapi/`: new `/signal/*` endpoints; `handleAlertmanager` refactored onto the shared routing core; auth extended with signal-adapter token validation.
- `internal/ingest/` (+ `routeGroup`): normalized-signal entry point; job-lane routing for cron-style sources.
- `internal/controller/`: `SignalAdapterReconciler` (+ shared workload-rendering helper extracted from `ChannelAdapterReconciler`); SignalSource `Served` condition.
- New `signal-cron/` module + Dockerfile/image.
- `chart/`: new CRD file, manager RBAC gains `signaladapters` (+status); chart minor bump.
- Live install: existing `alertmanager` SignalSource keeps working with no migration; Alertmanager webhook URL unchanged.
