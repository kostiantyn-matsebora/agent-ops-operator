## Why

Demo mode now defaults `global.agentops.wiringMode` to `coordinator`
(`coordinator-deployment-mode`, merged in #291, console claim fixed in
#296). The turnkey install renders one chart-managed `Coordinator` fanning
out to the kubernetes bundle's routes. Nothing renders a plain `Pipeline`
anymore.

A prospect running the turnkey demo now sees exactly one of the product's
two wiring primitives, the orchestrator. They never see the simpler one,
`Pipeline`, that most adopters will actually write first.

`docs/getting-started.md` papers over this today by pinning
`--set global.agentops.wiringMode=pipelines` on the install command, with a
callout explaining that coordinator mode left the console unwired. That
callout is now stale: #296 fixed the claim. But pinning `pipelines` means
the walkthrough shows only the OTHER primitive, never both.

Neither state of the demo today shows what the product actually offers:
Pipelines and Coordinators coexisting on one cluster, claiming shared
sources, answered differently.

## What Changes

- **The kubernetes bundle's demo-mode branch renders one extra, plain
  `Pipeline`** (`k8s-observe-pipeline` by default) alongside whatever
  `global.agentops.wiringMode` already renders for the bundle. A demo
  install then exhibits both primitives at once:
  - the chart-rendered `Coordinator`, fanning out to `AgentCapability`
    members and escalating to a human only when it decides to
  - a directly-addressed, always-replies-in-thread `Pipeline`, reusing the
    SAME `k8s-engineer` profile and observing toolset the coordinator's own
    `k8s-observe` capability uses, at the SAME privilege level

  The showcase costs no new identity, image, or credential.
- **New value `global.demo.wiringShowcase`**, nullable like
  `pipelines.enabled`. It governs the extra `Pipeline`:
  - unset under `global.demo.enabled: true`, it defaults to `true` ONLY
    when the resolved `wiringMode` is `coordinator` — the common demo path
  - explicit `false` declines it, leaving today's single-shape behavior
  - outside demo mode it has no effect at all

  A real install's `wiringMode` and its bundles' rendering stay UNCHANGED,
  byte-identical to before this change.
- **The showcase `Pipeline` claims the kubernetes bundle's own
  `cluster-events` source AND the console's chat source.** This is the same
  many-to-many sharing two Pipelines already support today, now demonstrated
  between a `Pipeline` and a `Coordinator` claiming one source at once.
  - A cluster `Warning` event now opens two conversations side by side in
    the console's Topology.
  - An unaddressed console message becomes a genuine two-claimant choice
    (`/k8s-coordinator` or `/k8s-observe-pipeline`). That choice-list
    behavior is already specified for a shared chat surface, not new
    ambiguity handling.
- **`docs/getting-started.md` drops the `wiringMode=pipelines` pin** and its
  now-stale callout. It installs with demo mode's own default, and walks
  through asking both the coordinator by name and the showcase pipeline by
  name.
- **`docs/guides/coordinate-agents.md`'s callout is corrected.** The console
  source and escalation channel ARE claimed for the chart-rendered
  `Coordinator` now (#296). What remains true — an ordinary, non-escalated
  reply does not land in a console thread, because a root's `channelRefs`
  stays empty until `escalate()` binds it — is restated as the deliberate
  difference between the two primitives' interaction styles, pointing at
  the showcase `Pipeline` as the always-visible counterpart.
- **No change to `wiringMode`'s own semantics, scope, or defaults.** It
  stays release-wide, two-valued, defaulting to `pipelines` (`coordinator`
  under demo). The showcase is an additive layer entirely inside
  `global.demo.*` and the kubernetes bundle's own demo branch.

**Explicitly not in scope:**

- No per-bundle `wiringMode` override, and no change to the prometheus or
  home-assistant bundles. Neither renders under demo mode today.
- No showcase for an operator who explicitly sets
  `global.agentops.wiringMode: pipelines` under demo. That install already
  shows the `Pipeline` primitive. Adding a demo-only `Coordinator` for the
  reverse case is left for a later change if anyone asks for it.
- No change to how `Coordinator` roots bind channels, or to `escalate()`.
  The showcase works WITH that behavior, not around it.

## Capabilities

### New Capabilities

None. The mechanism is a new rendering branch inside an existing
capability, not a new kind of object or contract.

### Modified Capabilities

- `k8s-bundle`: gains a demo-only requirement describing the showcase
  `Pipeline` — when it renders, what it claims, and that it is additive to
  (never a replacement for) whatever `wiringMode` already renders for the
  bundle.

## Impact

**Code**

- `chart/charts/kubernetes/values.yaml`: new `pipelines.demoShowcase` block
  (name, description, icon) read by the new template.
- `chart/charts/kubernetes/templates/`: a new template, or an added branch
  in `pipelines.yaml`, rendering the showcase `Pipeline`. Gated on
  `global.demo.wiringShowcase` resolving true and the bundle's resolved
  `wiringMode` being `coordinator`.
- `chart/values.yaml`: documents `global.demo.wiringShowcase` beside
  `global.demo.enabled`.
- `platform/manager/internal/integration/charttemplate_test.go`, or the
  kubernetes bundle's own render tests: new scenarios for the showcase
  Pipeline's rendering and non-rendering conditions.

**Docs — reference**

- `docs/configuration.md`: new row for `global.demo.wiringShowcase` beside
  the existing `global.agentops.wiringMode` row.
- `docs/concepts.md`: if it names demo mode's wiring posture, update to
  mention the showcase Pipeline.

**Docs — adopter site**

- `docs/getting-started.md`: drop the `wiringMode=pipelines` pin and its
  stale callout. Walk through both primitives instead.
- `docs/guides/coordinate-agents.md`: correct the "console does not
  auto-wire itself under this mode, yet" callout to reflect #296's fix,
  restate the remaining escalation-only channel behavior as a deliberate
  difference, and point at the showcase Pipeline as the always-in-thread
  counterpart.
- `.claude/rules/chart.md`: the kubernetes bundle's "wiring component" and
  "THE DEMO WIRES THE CONSOLE" sections gain the showcase branch.
