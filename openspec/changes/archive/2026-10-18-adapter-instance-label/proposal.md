## Why

Every `SignalAdapter`/`ChannelAdapter` instance's reconciler-owned Deployment
carries the same `app.kubernetes.io/name` value.

- Every signal adapter (`cron`, `alertmanager`, `k8s-events`, `ha`, the
  telegram signal adapter) renders `agentops-signal-adapter`.
- Every channel adapter (`console`, `telegram`) renders `agentops-adapter`.

Label-grouping tooling cannot tell distinct instances apart — Cilium Hubble's
service map, Grafana, `kubectl get pods -l ...`. Observed live: a Hubble
diagram rendering four identical "agentops-signal-adapter" boxes and two
identical "agentops-adapter" boxes, with no way to tell which instance is
which.

## What Changes

- Add `app.kubernetes.io/instance: <adapter CR name>` to the labels the
  `SignalAdapterReconciler` and `ChannelAdapterReconciler` stamp on their
  owned Deployment, Pod template, and (when rendered) Service.
- `app.kubernetes.io/name` is left untouched — `chart/templates/networkpolicy.yaml`
  already selects ingress traffic by that exact generic value for every
  adapter pod to reach the manager, and every other chart component (manager,
  console, gateway-telegram, housekeeping, reaper, coordinator, runtime)
  already uses `app.kubernetes.io/name` as a per-kind identifier consistently.
  Repurposing it for adapters alone would both break that NetworkPolicy and
  make the convention inconsistent chart-wide.
- `app.kubernetes.io/instance` is additive only: no existing Deployment,
  Service or NetworkPolicy selector reads it today, so there is no
  immutable-selector migration to perform.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `signal-adapter-lifecycle`: "Reconciler owns the workload and names its
  identity" gains the `app.kubernetes.io/instance` label on the Deployment
  (and Service, when rendered).
- `channel-adapter-lifecycle`: "Reconciler owns the adapter workload" gains
  the same label.

## Impact

- **Code**: `platform/manager/internal/controller/signaladapter_controller.go`,
  `platform/manager/internal/controller/channeladapter_controller.go`, and
  their unit tests.
- **Reference docs**: none of `docs/concepts.md`, `docs/contracts.md`, or any
  bundle page documents the Kubernetes object labels a reconciler stamps on
  an adapter's Deployment today — this change adds metadata no reference
  page currently claims, so none goes stale. `docs/contracts/signal-adapter.md`
  and `docs/contracts/channel-adapter.md` govern the HTTP/webhook protocol
  between adapter and manager, not Kubernetes object labels, so neither is
  touched — `.claude/rules/signal-rules.md` also requires explicit user
  approval before editing the signal-adapter spec, which this change does
  not seek.
- **Adopter site**: no page (landing, getting-started, installation, guides)
  states or depends on this label — nothing goes stale.
- **CHANGELOG.md**: a one-line `### Added` entry under the next manager
  release, since it is an observable (if non-breaking) change to rendered
  objects.
