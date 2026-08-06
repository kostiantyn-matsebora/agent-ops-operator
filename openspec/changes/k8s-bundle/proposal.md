# k8s-bundle

## Why

The pieces of a "Kubernetes engineer agent" experience exist only as scattered fragments: the `k8s-engineer` AgentProfile, its runtime, SA, and read-only RBAC live inside the demo template; the planned k8s-events signal adapter (change `k8s-events-signal-source`, merged into this change) had no profile to point its default SignalSource at; and there is no single switch that turns on "watch my cluster and let an agent act on what it sees". A `k8s-bundle` subchart packages all three — events signal source, k8s-engineer profile (with runtime + SA), and its RBAC (read-only by default, full opt-in) — as one composable, individually-toggleable unit that demo mode simply enables.

## What Changes

- New Helm subchart `chart/charts/k8s-bundle/` with three individually enablable components (each with its own flag), the whole bundle disabled by default and auto-enabled by demo mode with read-only RBAC:
  - **`eventsAdapter`**: the k8s-events `SignalAdapter` CR (`type: k8sEvents`, SA token automount), its Events `get/list/watch` RBAC, and a default `SignalSource` wired to the bundle's `k8s-engineer` profile (severities default `["Warning"]`) — resolving the profile chicken-and-egg the standalone change had.
  - **`profile`**: the `k8s-engineer` AgentProfile plus its execution identity — an `AgentRuntime` (values-configured image + LLM credentials Secret ref, name defaults to `default` preserving today's demo behavior) and a dedicated runtime ServiceAccount.
  - **`rbac`**: bindings for that ServiceAccount — `mode: readonly` (built-in `view` + nodes/namespaces/metrics reads, today's demo RBAC) or `mode: full` (**cluster-admin**), or disabled entirely.
- Everything absorbed from the merged `k8s-events-signal-source` change (that change is retired): the `signal-k8s-events/` adapter module (dependency-free in-cluster Events watcher, configurable severities defaulting to Warning), `SignalAdapterSpec.automountServiceAccountToken` opt-in (default false, ChannelAdapter rendering byte-identical), `SourceK8sEvents` constant, image build target.
- **BREAKING (chart values)**: the demo toggle moves to `global.demo.enabled` (subcharts can't see parent-scoped values); `demo.runtimeImage`, `demo.credentialsSecret`, and `demo.readOnlyRbac` move into `k8s-bundle.*` values; `chart/templates/demo.yaml` is deleted (fully absorbed by the bundle). Chart major version bump.
- README/samples updated: demo instructions now describe the bundle; standalone bundle usage documented (`k8s-bundle.enabled=true` without demo).

## Capabilities

### New Capabilities

- `k8s-events-signal-adapter`: the Kubernetes-events signal adapter (carried over from the merged change) — config schema, event normalization/fingerprints, restart-safe watching; chart packaging now lives in the bundle capability.
- `k8s-bundle`: the subchart composition — component set and per-component flags, default-off/demo-on enablement, RBAC modes (readonly default, full opt-in), runtime/SA identity wiring, and the values migration from the old demo block.

### Modified Capabilities

- `signal-adapter-lifecycle`: (carried over from the merged change) the zero-ambient-authority posture becomes the default rather than absolute — `SignalAdapterSpec` gains opt-in `automountServiceAccountToken` for adapters whose data source is the Kubernetes API; the manager still never creates RBAC; ChannelAdapter rendered output MUST remain byte-identical.

## Impact

- **New module**: `signal-k8s-events/` (unchanged scope from the merged change).
- **API**: `automountServiceAccountToken` on `SignalAdapterSpec` + `SourceK8sEvents` constant; deepcopy + CRD regen.
- **Controller**: automount threading in `adapterworkload.go` / `signaladapter_controller.go`; ChannelAdapter rendering pinned unchanged.
- **Chart**: new `chart/charts/k8s-bundle/` subchart (templates: adapter CR + RBAC, SignalSource, AgentProfile, AgentRuntime, SA, role bindings); parent `values.yaml` gains `global.demo.enabled` + `k8s-bundle` override block; `templates/demo.yaml` deleted; chart version → 2.0.0.
- **Docs**: README (bundle concept, demo section rewrite, values migration table), CLAUDE.md (module map, build commands), `config/samples/samples.yaml`.
- **Merged change**: `openspec/changes/k8s-events-signal-source/` is superseded and removed; its spec deltas are re-issued from this change.
