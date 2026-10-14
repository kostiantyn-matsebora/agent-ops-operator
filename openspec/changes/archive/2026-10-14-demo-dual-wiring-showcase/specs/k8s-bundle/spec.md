## ADDED Requirements

### Requirement: Demo mode renders a showcase Pipeline alongside coordinator mode

When the bundle's demo-mode branch resolves `global.agentops.wiringMode` to
`coordinator`, the bundle SHALL ALSO render one plain `Pipeline` — the
showcase route — alongside the chart-rendered `Coordinator`'s own
`AgentCapability` entries, so a demo install exhibits both wiring
primitives at once rather than the Coordinator alone.

The showcase route SHALL reuse the SAME `k8s-engineer` profile and the SAME
observing toolset as the bundle's `k8s-observe` capability, at the SAME
privilege level, so it needs no new identity, image, or credential.

The showcase route SHALL claim the bundle's own `cluster-events` source and
the console's chat source, on the SAME terms any `pipelines`-mode route
claims them today — sources remain shareable, and the Coordinator's own
claim on the same sources is unaffected.

Rendering the showcase route SHALL be governed by `global.demo.wiringShowcase`
(nullable):

- Unset, under `global.demo.enabled: true` and a resolved `wiringMode` of
  `coordinator`, it SHALL default to rendering the showcase route.
- An explicit `false` SHALL decline it, in both directions — even under
  demo mode.
- Outside `global.demo.enabled: true`, or where the resolved `wiringMode`
  is `pipelines`, it SHALL render nothing: the bundle's ordinary
  `pipelines`-mode route already supplies the `Pipeline` primitive, and
  adding a second one would duplicate it for no reason.

A release that sets neither `global.demo.enabled` nor
`global.agentops.wiringMode` SHALL render byte-identical to a release
before this requirement existed: the showcase route depends on demo mode's
own default resolving to `coordinator`, and nothing here changes
`wiringMode`'s own resolution or `pipelines.yaml`'s existing rendering.

#### Scenario: Demo mode's default renders both primitives

- **WHEN** the chart is installed with `global.demo.enabled=true` and
  nothing else
- **THEN** the chart-rendered `Coordinator` and its `AgentCapability`
  entries render, AND the kubernetes bundle also renders exactly one plain
  `Pipeline` claiming the same `cluster-events` source

#### Scenario: The showcase is declined explicitly

- **WHEN** the chart is installed with `global.demo.enabled=true` and
  `global.demo.wiringShowcase=false`
- **THEN** the chart-rendered `Coordinator` renders as before, and the
  bundle renders no additional `Pipeline`

#### Scenario: Explicit pipelines mode under demo renders no showcase

- **WHEN** the chart is installed with `global.demo.enabled=true` and
  `global.agentops.wiringMode=pipelines`
- **THEN** the bundle renders its ordinary `pipelines`-mode `Pipeline` and
  no `Coordinator`, `AgentCapability`, or second `Pipeline` from this
  requirement

#### Scenario: A real install is unaffected

- **WHEN** the chart is installed with `global.demo.enabled` unset or
  `false`
- **THEN** no object rendered by this requirement appears, whatever
  `global.agentops.wiringMode` is set to

#### Scenario: The showcase route shares the console and the cluster source

- **WHEN** the showcase route renders
- **THEN** it claims the kubernetes bundle's `cluster-events` source and
  the console's chat source exactly as a `pipelines`-mode route would, and
  the Coordinator's own claim on those same sources is unchanged

#### Scenario: The showcase route costs no new substrate

- **WHEN** the showcase route renders
- **THEN** it names the bundle's own `k8s-engineer` profile and observing
  toolset, renders no new `AgentRuntime`, ServiceAccount, or credential, and
  runs at the same privilege level as the bundle's `k8s-observe` capability
