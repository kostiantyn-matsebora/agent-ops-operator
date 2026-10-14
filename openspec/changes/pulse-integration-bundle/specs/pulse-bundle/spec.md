## Purpose

The Pulse Helm subchart composition at `chart/charts/pulse/`. Packages the
webhook signal adapter, the first-party MCP server with its two risk-split
tool pairs, two AgentProfiles, and the bundle's own default-off wiring, in
both Pipeline mode and Coordinator mode.

## ADDED Requirements

### Requirement: The bundle is self-gated and named for the product, off by default
The bundle SHALL be named `pulse`, matching `kubernetes`, `prometheus`,
`home-assistant` and `telegram`'s unsuffixed naming.

It SHALL default off and SHALL NOT be enabled by demo mode. No demo
cluster runs a Pulse instance for the bundle to reach.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, MCPConfig, MCPToolset,
  AgentProfile, Pipeline, AgentCapability, route ServiceAccount or workload
  from the bundle is rendered

#### Scenario: Demo mode does not enable this bundle
- **WHEN** the chart is installed with `global.demo.enabled=true` and
  default bundle values
- **THEN** no object from this bundle renders

### Requirement: The ingest component packages the adapter with its default source
When active, the `signal` component SHALL render the Pulse `SignalAdapter`
CR and, under `defaultSource.enabled`, a `SignalSource` naming it.

The claim on that source SHALL belong to the wiring component or to the
install, never to the ingest component itself — an ingest lane can be
rendered for a route declared anywhere.

#### Scenario: The ingest lane renders without a claim
- **WHEN** the ingest component is active and neither the wiring component
  nor the install claims its source
- **THEN** the adapter and the source render, and the source reports
  `Wired=False` and drops its alerts

### Requirement: The bundle ships two AgentProfiles, framed for Community edition
When active, the `profiles` component SHALL render two AgentProfiles:
`pulse-observer` and `pulse-operator`. Neither carries a repository, so
each SHALL ship an inline role.

Both roles SHALL state plainly that Pulse's governed-action
plan/approve/execute flow is Pro-gated and out of reach on Community
edition. `pulse-observer`'s role SHALL frame investigation only.
`pulse-operator`'s role SHALL additionally frame the operator-state
annotations (maintenance windows, criticality tags) as the only mutating
actions available, never as a stand-in for an actual fix.

#### Scenario: Both profiles render with no capabilities of their own
- **WHEN** the profiles component renders
- **THEN** neither `AgentProfile` declares `allowedTools` or `mcp` —
  capability binding is the wiring component's job

#### Scenario: The operator role never claims to execute a governed fix
- **WHEN** an operator reads the `pulse-operator` profile's inline role
- **THEN** it states that it may annotate (maintenance window, criticality)
  but cannot plan, approve or execute a Pulse action

### Requirement: The wiring component renders both routes, in both wiring modes
Under `global.agentops.wiringMode: pipelines`, the wiring component SHALL
render two Pipelines, one per profile.

Each SHALL claim the bundle's alert source and bind its own
`MCPConfig`/`MCPToolset` pair, with values-supplied `channelRefs` omitted
when empty.

Under `wiringMode: coordinator`, the SAME two routes SHALL render as
AgentCapabilities instead, claimed by the chart-rendered Coordinator via
this bundle's own contribution helper — mirroring `prometheus-bundle`'s
`pipelines.yaml`/`capabilities.yaml` split.

Each route SHALL be independently toggleable. A route referencing a
disabled sibling component (the ingest lane, either MCP pair) SHALL omit
that reference rather than dangle.

#### Scenario: Enabling the bundle adds no route by itself
- **WHEN** the bundle is enabled with default values
- **THEN** no Pipeline or AgentCapability renders, and the source reports
  `Wired=False`

#### Scenario: Pipelines-mode wiring claims the shared source twice
- **WHEN** both routes are enabled under `wiringMode: pipelines`
- **THEN** both Pipelines claim the same alert source — sources are
  shareable, so both render without conflict

#### Scenario: Coordinator-mode wiring contributes both capabilities
- **WHEN** both routes are enabled under `wiringMode: coordinator`
- **THEN** the chart-rendered Coordinator's `agents[]` lists both,
  described well enough that a person's question matches one as readily as
  an alert does

### Requirement: No substrate is rendered
The bundle SHALL render no `AgentRuntime`, no floor ServiceAccount, no
runtime credential and no context-volume claim. The parent chart's
`global.agentops.runtimeDefaults` and `runtimes:` own all of that, exactly
as every other bundle.

It SHALL render one ServiceAccount per route it ships (unless the install
names one), bound no Kubernetes RBAC by default.

#### Scenario: The route's identity is the bundle's, the substrate is not
- **WHEN** a wiring component route renders with no `serviceAccountName`
  set
- **THEN** a ServiceAccount for that route renders with no RBAC bound,
  while the runtime defaults, the floor account and the context volume
  still come from the parent chart

### Requirement: This bundle's deployed MCP server is first-party, unlike every prior bundle's
Where `kubernetes` and `prometheus` reference a third party's published MCP
server image, and `home-assistant` points at a vendor-served endpoint,
`pulse`'s `mcp-pulse` component SHALL deploy an image this repository
itself builds and publishes (`agentops-mcp-pulse`).

The design document SHALL record this as the first such precedent, so a
later bundle facing the same "no deployable vendor server" gap has it to
follow.

#### Scenario: The server image is this repository's own
- **WHEN** the mcp-pulse component renders
- **THEN** its image repository is `agentops-mcp-pulse`, built from
  `platform/mcp-pulse/` by this repository's own release pipeline
