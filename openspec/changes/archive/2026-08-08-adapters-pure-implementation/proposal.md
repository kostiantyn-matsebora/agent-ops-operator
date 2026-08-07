# Proposal: adapters-pure-implementation

## Why

User architectural correction: adapters are **implementations**, Channels/SignalSources are **configuration and appliance**. The current model leaks both ways — adapter CRs carry a free-form `env` configuration surface, the vm-bundle chart carries the webhook's connectivity (port + Service), and adapters carry a `type` field that is redundant once implementation↔adapter is recognized as 1:1 (two adapters for one implementation make no sense; today that's merely guarded by `TypeConflict`).

## What Changes

- **BREAKING** — `ChannelAdapter`/`SignalAdapter` lose `spec.type` and `spec.env`:
  - the adapter **CR name is the routing key**: `Channel.spec.type` / `SignalSource.spec.type` name the serving adapter (or in-process registry key). Duplicate adapters per implementation become structurally impossible; the `TypeConflict` guard and `olderClaimant` logic are deleted. Derived tokens are scoped to the adapter's name (= the key it serves). The contract's `?type=` query parameter keeps its name — it's the same key string.
  - no configuration on implementations: `env` is gone (the secret-env guard with it); per-surface settings stay in the served CRs' `config`/`credentialsSecretRef`; implementation-intrinsic env is injected by the reconciler or baked into the image.
- `SignalAdapter.spec.port` (optional, implementation property): when set, the reconciler owns a Service `agentops-signal-<name>` on that port and injects `LISTEN_ADDR` — enabling an adapter is complete by itself. The vm-bundle chart drops its Service template and `service.port` value; the bundle's SignalSource `type` renders from the adapter name so they can never drift.
- `SourceVMAlertmanager` constant removed — the manager references no specific signal types; the key is operator-chosen.

## Capabilities

### Modified Capabilities

- `channel-adapter-lifecycle`: CRD shape (no type/env), name-as-key, no TypeConflict.
- `signal-adapter-lifecycle`: same + reconciler-owned Service for port-declaring adapters.
- `channel-adapter-contract` / `signal-adapter-contract`: token scope wording — scoped to the key the adapter serves (its name).
- `vm-bundle`: the alertmanager component ships no Service; connectivity comes from the adapter's `port`.
- `vm-alertmanager-signal-adapter`: `LISTEN_ADDR` reconciler-injected; source type = adapter name in the bundle.

## Impact

- `api/v1alpha1` (both adapter specs, printcolumns), both adapter reconcilers + shared workload helper, httpapi auth scoping, Channel/SignalSource Served lookups, tests, vm-bundle + telegram chart templates, samples, docs; CRD regen; chart 1.7.0 / manager 0.9.0.
- Live: telegram already has name==type (no-op). `vm-alerts` has immutable `type: vmAlertmanagerWebhook` ≠ adapter name → recreated as part of the already-gated cutover (source is fresh, nothing lost).
