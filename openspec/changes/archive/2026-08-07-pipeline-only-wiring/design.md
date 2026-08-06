# Design: pipeline-only-wiring

## Context

`wire-it-up` deliberately kept the old per-CR refs as a fallback (pipeline-first resolution) so the upgrade was behavior-neutral. The user's call: now that Pipeline exists and is live, the fallback is duplication — remove it. Wiring facts (which sources feed which profile, which channels mirror, what answers bare messages) live ONLY on Pipeline; `Conversation.spec.profileRef`/`channelRefs` remain as materialized per-conversation state, not wiring.

## Goals / Non-Goals

**Goals:** one place to declare wiring; unwired objects fail loudly (conditions + response reasons), never silently.
**Non-Goals:** no changes to Pipeline semantics (claiming, mirroring, delivery); no per-signal routing overrides; `/task`'s explicit `profile`/`channel`/`pipeline` params stay (that's caller intent, not standing wiring).

## Decisions

- **D1: Field removals** — `SignalSourceSpec.ChannelRef`, `SignalSourceSpec.ProfileRef`, `ChannelSpec.DefaultProfileRef` deleted (BREAKING; API is pre-1.0, and the fields' values are pruned from stored objects on next write once the CRD drops them — live manifests get a one-time cleanup).
- **D2: Unwired sources drop signals visibly** — `routeSignals` resolves the Ready-Pipeline claim first and returns a `reason` when unclaimed; both entry points surface it (`{queued: 0, reason: "source not claimed by a Ready pipeline"}`). `SignalSourceReconciler` adds a `Wired` condition (True + pipeline name / False), watching Pipelines — `kubectl get signalsource` answers "why is nothing happening" without reading logs.
- **D3: Channel default profile = its oldest Ready Pipeline's profile** — the router's `defaultProfile` already resolves this; the `defaultProfileRef` branch is deleted. Bare messages on unwired channels keep the existing warning send; commands and thread replies work everywhere regardless.
- **D4: Migration ordering** — apply the Pipeline CR before upgrading the manager (old manager ignores Pipelines; new manager requires the claim), so there is no window where alerts drop. Documented as the upgrade sequence; the wire-it-up live install already validated pipeline routing.

## Risks / Trade-offs

- [A forgotten Pipeline silently... no — loudly — stops a source] → that's the point: `Wired=False` + response reason; release notes lead with the migration step.
- [Chat-less signal sources (no channels wanted)] → a Pipeline with `channelRefs: []` is valid — profile-only wiring.
- [Test churn] → helper signatures shrink; signal tests gain a tiny mkPipeline+reconcile step.

## Migration Plan

1. Apply a Pipeline claiming every live source with the intended profile/channels (`home-ops-pipeline`: alertmanager → ha-engineer → home-ops).
2. Upgrade (CRDs drop the fields; manager 0.7.0 routes pipeline-only).
3. Re-apply cleaned CR manifests (fields gone). Rollback: previous chart + manifests.

## Open Questions

- None — scope pinned by the user's instruction.
