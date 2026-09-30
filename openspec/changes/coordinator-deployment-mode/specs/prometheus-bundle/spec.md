## Purpose

The Prometheus/Alertmanager Helm subchart composition at
`chart/charts/prometheus/`. It packages the Alertmanager webhook
signal adapter, one Prometheus query MCP configuration with its deployable
server, the `alert-investigator` profile, and the bundle's own default-off
wiring.

It ships no execution substrate — the runtime, its ServiceAccount, its
credential and that SA's RBAC are the parent chart's
(`agent-runtime-ownership`). Self-gated, off by default, and never enabled
by demo mode.

## MODIFIED Requirements

### Requirement: The wiring component ships one claiming Pipeline, off by default
The bundle SHALL offer a wiring component rendering a `Pipeline` that claims
the bundle's own alert source and names the bundle's own profile, binding
the bundle's metrics toolset and `MCPConfig`.

`pipelines.enabled` SHALL default to `false`. Unlike the Kubernetes bundle,
NO values path SHALL force it on, because no turnkey mode enables this
bundle at all.

The component SHALL render only when the profile component renders, since a
Pipeline with no profile has no agent to run.

Exactly ONE route SHALL be offered. A metrics query server is read-only, so
there is no second posture to express.

The component SHALL render the ServiceAccount that route executes under, unless
the install names one, and SHALL bind it no Kubernetes RBAC by default.

Every reference the Pipeline makes to an object the bundle does not itself render
SHALL be a values-supplied name, omitted when unset. Channels SHALL be such a
list and SHALL default to empty.

With none bound, the conversation dispatches without waiting and its answer
is readable from `status.runs[].result`. A ref to a bundle component that is
turned off SHALL be omitted rather than dangling.

Rendering alongside an install-declared Pipeline claiming the same source SHALL
be possible and SHALL NOT fail: sources are shareable. It SHALL be reported in
the post-install notes for what it is — one alert opening two conversations,
under two profiles, with two agents acting.

Under `global.agentops.wiringMode: coordinator`, the bundle SHALL render its
SAME route identity as a standalone `AgentCapability` rather than an inline
Pipeline — the `alert-investigator` capability, bound to the same toolset
and `MCPConfig` the Pipeline route binds today. The bundle SHALL NOT render
its own `Pipeline` in this mode.

Since this bundle offers exactly one route, there is exactly one
`AgentCapability` to render, and no turnkey mode forces this bundle's
wiring on in either posture.

#### Scenario: Enabling the bundle adds no route by itself
- **WHEN** the bundle is enabled with default values
- **THEN** no `Pipeline` renders, the source reports `Wired=False`, and the
  install's own `pipelines:` remain the only routes

#### Scenario: The wiring flag yields an install that answers
- **WHEN** the wiring flag is turned on with the profile and ingest components
  active
- **THEN** exactly one `Pipeline` renders, claiming the bundle's source with the
  bundle's profile, toolset and MCPConfig, and an admitted alert opens a
  conversation with no further configuration

#### Scenario: Wiring without a profile
- **WHEN** wiring is enabled with the profile component off
- **THEN** no Pipeline renders, because a Pipeline with no profile has no agent
  to run

#### Scenario: A channel is named
- **WHEN** the wiring component's channel list names an existing Channel
- **THEN** the rendered Pipeline carries that `channelRefs` entry — with the
  list empty the field is absent, not empty-valued

#### Scenario: A disabled component leaves no dangling reference
- **WHEN** wiring renders with the metrics component turned off
- **THEN** the Pipeline omits the toolset and MCPConfig refs entirely rather than
  naming objects nobody created

#### Scenario: The install also claims the source
- **WHEN** the bundle's wiring is active and an install-declared Pipeline also
  lists the bundle's alert source
- **THEN** both render, and the post-install notes state that each alert now
  opens two conversations

#### Scenario: Coordinator mode renders a capability, not a Pipeline

- **WHEN** `global.agentops.wiringMode: coordinator` is set and the bundle's
  wiring is enabled
- **THEN** the bundle renders the `alert-investigator` `AgentCapability`
  bound to the metrics toolset and `MCPConfig`, and renders no `Pipeline` of
  its own

#### Scenario: Coordinator mode's capability is claimed by the chart Coordinator

- **WHEN** coordinator mode is set and the bundle's wiring is enabled
- **THEN** the chart-rendered Coordinator's `agents[]` lists this bundle's
  `AgentCapability`, and its `signalSourceRefs` claims the bundle's alert
  source — so the source is not left `Wired=False`

#### Scenario: No turnkey mode forces this bundle's wiring on, in either posture

- **WHEN** `global.demo.enabled=true` and `global.agentops.wiringMode:
  coordinator` are both set with the bundle otherwise at defaults
- **THEN** the bundle's wiring stays off and no `AgentCapability` or
  `Pipeline` renders, exactly as under `pipelines` posture
