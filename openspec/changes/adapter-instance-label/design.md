## Context

See `proposal.md` — Why. Both reconcilers build a `labels` map before calling
`ensureAdapterWorkload`/`ensureAdapterService` (`adapterworkload.go`):

- `signaladapter_controller.go:104` — `labels := map[string]string{"app.kubernetes.io/name": "agentops-signal-adapter", signalAdapterLabelKey: adapter.Name}`
- `channeladapter_controller.go:105` — same shape, `"agentops-adapter"` / `channelAdapterLabelKey`

`signalAdapterLabelKey` (`agentops.dev/signal-adapter`) and
`channelAdapterLabelKey` (`agentops.dev/adapter`) already carry the CR name
and are used as the Deployment/Service `SelectorKey`. They are not touched by
this change — they stay the selector mechanism.

## Goals / Non-Goals

**Goals:**
- Add `app.kubernetes.io/instance: <adapter.Name>` to the `labels` map both
  reconcilers already build, so it lands on the Deployment, its Pod template,
  and the Service (when rendered) for free — all three already read from the
  same map.

**Non-Goals:**
- Changing `app.kubernetes.io/name`, the selector keys, or anything
  NetworkPolicy/Service selects by.
- A chart-level change — the label is reconciler-owned, not chart-rendered.
- Retrofitting the generic `app.kubernetes.io/name` labels already set on
  the manager, console CR metadata, gateway, housekeeping, reaper or
  coordinator workloads — out of scope for this change.

## Decisions

**Add `app.kubernetes.io/instance`, don't repurpose `app.kubernetes.io/name`.**
`networkpolicy.yaml:48,51` selects ingress by the current generic
`app.kubernetes.io/name` values. Changing what that label means for adapters
would break that policy silently.

The `Deployment`/`Service` `spec.selector` is also immutable — any
selector-key change would force a delete-and-recreate migration.
`app.kubernetes.io/instance` is additive: it has no existing reader, so it
ships with zero migration risk.

**Use the CR name as the value, not a compound string.** Considered a
compound `component` label like `console-channel-adapter`, but
`app.kubernetes.io/instance` already is the Kubernetes-recommended field for
"which specific one."

Its value should match what `AdapterDeploymentName`/
`SignalAdapterDeploymentName` already derive the object's own name from —
so `instance: console` correlates directly with `agentops-adapter-console`.

**No `app.kubernetes.io/component` label added.** The existing
`app.kubernetes.io/name` values (`agentops-adapter`,
`agentops-signal-adapter`) already carry the role, consistent with how every
other chart component (`agentops-manager`, `agentops-console`,
`agentops-gateway-telegram`, ...) uses `app.kubernetes.io/name` as a
per-kind identifier. Adding `component` on top would duplicate that
without fixing anything `instance` alone doesn't already fix.

## Risks / Trade-offs

**A pre-existing live Deployment/Service won't get the new label until its
next reconcile.** → `ensureAdapterWorkload`/`ensureAdapterService` already
patch on every reconcile (source/channel changes roll the pod).

This self-heals on the next change to any served source/channel, or an
operator restart/resync. No forced rollout needed since no selector changes.

**The console's dual identity (one Deployment serving both a ChannelAdapter
and a `servedBy` SignalAdapter) only gets one `instance` value** — the
owning ChannelAdapter's name (`console`). → Acceptable: the served
SignalAdapter identity is already env-var-only (`SIGNAL_ADAPTER_NAME`), not
labeled, and this change doesn't alter that.
