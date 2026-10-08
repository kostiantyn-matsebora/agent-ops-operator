## Context

`global.agentops.wiringMode` (`coordinator-deployment-mode`, #291) is a
release-wide, two-valued rendering posture. The helper
`agentops.wiringMode` in `chart/templates/_helpers.tpl` resolves it. An
explicit value wins. Otherwise `global.demo.enabled: true` resolves
`coordinator`, and everything else resolves `pipelines`.

The kubernetes bundle's two gating templates already follow this resolved
value exclusively:

- `chart/charts/kubernetes/templates/pipelines.yaml` renders a `Pipeline`
  per route under `pipelines` mode.
- `chart/charts/kubernetes/templates/capabilities.yaml` renders an
  `AgentCapability` per route under `coordinator` mode, claimed by the
  parent's chart-rendered `Coordinator` (`chart/templates/coordinator.yaml`,
  `agentops.coordinatorContributions`).

Both read the SAME per-route gating helpers
(`kubernetes.observePipelineEnabled`, `kubernetes.adminPipelineEnabled`,
`kubernetes.wiringActive`), so the two cannot drift on which route exists.

Demo mode's own default is already `coordinator`. This proposal adds a
THIRD, additive branch. It fires only under demo mode, only when the
resolved mode is `coordinator`, and only for the observing route. See
`proposal.md` for why.

## Goals / Non-Goals

**Goals:**

- One new template renders exactly one extra `Pipeline` under the stated
  conditions, sharing the observing route's profile, toolset, and
  privilege level.
- The new rendering is reachable through the SAME helper pattern
  (`kubernetes.*Enabled`) the other two files use, so a later change to
  the observing route's definition (tools, icon, description) cannot drift
  between the three.
- `global.demo.wiringShowcase` follows the SAME nullable convention as
  `pipelines.enabled`: unset derives, explicit wins both ways.

**Non-Goals:**

- No shared Helm partial extracted across the three gating files. They
  already tolerate controlled duplication, each annotated with a
  cross-reference warning the others not to drift.
  `capabilities.yaml`'s own header is the precedent. A fourth near-copy
  follows the established convention instead of a new abstraction for
  three call sites.
- No admin-route showcase. The observing route alone makes the point.
  Doubling the acting route would also double the mutating toolset's
  exposure in a demo cluster, for no demonstration value.
- No change to `coordinator.yaml`, `reaper.yaml`, `coordination.yaml`, or
  any other bundle. The mechanism is entirely inside
  `chart/charts/kubernetes/`.

## Decisions

### A new template file, not a third branch inside `pipelines.yaml`

`pipelines.yaml`'s own header states "UNDER coordinator mode THIS FILE
RENDERS NOTHING" as a deliberate, load-bearing invariant. Every reader of
that file can assume coordinator mode means the file is inert.

Threading a coordinator-mode-only branch into it would contradict its own
comment. A new file,
`chart/charts/kubernetes/templates/pipeline-showcase.yaml`, keeps each
file's gating condition a single, legible line.

### A new helper, mirroring the existing nullable pattern

`kubernetes.demoShowcaseActive` mirrors `kubernetes.wiringActive` exactly:

```
{{- define "kubernetes.demoShowcaseActive" -}}
{{- $g := .Values.global | default dict -}}
{{- if kindIs "bool" (dig "demo" "wiringShowcase" "" $g) -}}
{{- if dig "demo" "wiringShowcase" false $g }}true{{ end -}}
{{- else if dig "demo" "enabled" false $g -}}
true
{{- end -}}
{{- end -}}
```

An explicit boolean wins in both directions, including `false` under demo
mode. That is the same contract `pipelines.enabled`'s own comment states
for itself.

Unset, it follows `demo.enabled` alone. The template file ANDs this with
`eq (include "agentops.wiringMode" .) "coordinator"`, so an explicit
`wiringMode: pipelines` under demo never renders the showcase, even though
`demoShowcaseActive` itself would say true.

The AND lives in the template, not the helper. The helper answers "did
demo ask for the showcase." It never answers "does the showcase make
sense right now." That second question is the template's — the same
division `kubernetes.wiringActive` and the per-route `*PipelineEnabled`
helpers already keep separate.

### The showcase route clones the observing route's shape, not a new route

It reads `pipelines.observe.*` for its profile binding, toolset, and
privilege level. It adds exactly one new value,
`pipelines.demoShowcase.name` (default `k8s-observe-pipeline`), plus
`.description` and `.icon` — the one thing that must differ: its own
object name, so it never collides with the `AgentCapability` of the same
underlying route.

Cloning the full route definition — its own `enabled`, `rbac`,
`runtimeRef` — would let an operator configure a route that no longer
matches what it showcases. The whole point is that it is the SAME route,
rendered twice.

### It claims sources exactly as `pipelines.yaml` does

`cluster-events` plus the console's signal source and channel, reusing
the same `global.agentops.console` read. Nothing new to decide here. The
sharing is the existing many-to-many mechanism (`wiring.md`), exercised
for the first time between a `Pipeline` and a `Coordinator` claiming one
source.

### No de-duplication against the Coordinator's own claim

The Coordinator claims `cluster-events` and the console source through
its own, independent read of `global.agentops.console` and
`kubernetes.coordinatorContribution`. The showcase route claims the same
names through `pipelines.yaml`'s existing pattern.

Both render. The API server fans the source out to both, exactly as the
`k8s-bundle` spec's existing "Both routes are asked for" scenario already
describes for two Pipelines. Suppressing one claim because the other
exists would be the retired `sourceConflicts` guard returning by a side
door.

## Risks / Trade-offs

**[Risk] A demo install now opens TWO conversations per cluster `Warning`
event instead of one.** This could read as a bug to someone who has not
read this change.

→ Mitigation: NOTES.txt already states this for the "both routes" case,
`pipelines.yaml`'s existing behavior. Extend the same notice to name the
showcase route specifically, and say why.

**[Risk] An unaddressed console message now surfaces a two-way choice**
(`/k8s-coordinator` or `/k8s-observe-pipeline`) on a fresh demo install.
That adds one extra step before a first-time user gets an answer.

→ Mitigation: this is the intended demonstration, and the easiest place
to point it out is `docs/getting-started.md`.

→ The walkthrough addresses each primitive by name instead of leaving the
first message unaddressed. The choice list is shown once, deliberately,
rather than hit by accident.

**[Risk] Existing chart render tests assert demo mode's default manifest
shape with no explicit `wiringMode` override.** They will see one new
`Pipeline` appear.

→ Mitigation: a task enumerates and updates those scenarios rather than
leaving them to fail CI first.

→ The five tests already pinned to `wiringMode=pipelines` after
`coordinator-deployment-mode` shipped are UNAFFECTED. They explicitly
override the mode away from `coordinator`, so the showcase never renders
there.

**[Risk] Two adopter pages describe a console-wiring gap #296 already
closed** (`docs/getting-started.md`, `docs/guides/coordinate-agents.md`).

→ Mitigation: corrected as part of this change's documentation task,
scoped to the paragraphs that are now factually wrong, not a rewrite of
either page.

## Migration Plan

Purely additive and demo-scoped. No existing object changes shape, no CRD
changes, and no value is renamed.

1. Add `pipelines.demoShowcase` to `chart/charts/kubernetes/values.yaml`
   and `global.demo.wiringShowcase` to `chart/values.yaml`'s documented
   demo block.
2. Add the helper and the new template.
3. Update the render-test scenarios that pin demo mode's default shape.
4. Update `docs/getting-started.md`, `docs/guides/coordinate-agents.md`,
   `docs/configuration.md`, and `.claude/rules/chart.md`.

Rollback is deleting the new template, helper, and values keys. Nothing
depends on them existing once rendered.

The showcase `Pipeline` is an ordinary chart-managed object with no
finalizer. Removing it orphans nothing beyond the ordinary conversations
it already started, the same lifecycle as any other `Pipeline` leaving a
release.
