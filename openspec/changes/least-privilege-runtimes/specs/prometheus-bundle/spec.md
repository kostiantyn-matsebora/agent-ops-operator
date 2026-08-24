## MODIFIED Requirements

### Requirement: The bundle ships as a self-gated subchart named for the protocol, not a vendor
The bundle SHALL be named `prometheus`, dropping the `-bundle` suffix, matching
`kubernetes`, `home-assistant` and `telegram`.

Naming it for the PROTOCOL rather than a vendor is unchanged, and is why the name
works: it reads the STANDARD Alertmanager payload, which vanilla Alertmanager and
VictoriaMetrics both send.

It SHALL remain self-gated and off by default, and demo mode SHALL still not
enable it. **Every retired key SHALL FAIL the render**, naming its replacement.

#### Scenario: The bundle is named for the protocol, without a suffix
- **WHEN** an install enables the Prometheus bundle
- **THEN** it does so under a key naming the protocol, with no suffix

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, MCPConfig, MCPToolset, AgentProfile, Pipeline, route ServiceAccount or workload from the bundle is rendered

#### Scenario: The route's identity is the bundle's, the substrate is not
- **WHEN** the wiring component renders with no `serviceAccountName` set on it
- **THEN** a ServiceAccount for that route is rendered with no Kubernetes RBAC bound to it, while the runtime defaults, the floor account and the context volume still come from the parent chart

#### Scenario: Demo mode does not enable this bundle
- **WHEN** the chart is installed with `global.demo.enabled=true` and default bundle values
- **THEN** no object from this bundle renders, because it consumes endpoints a demo cluster does not have

#### Scenario: The retired values key fails the render
- **WHEN** an install upgrades while still carrying a retired key for this bundle
- **THEN** the render FAILS naming the replacement, rather than succeeding with a bundle that silently renders nothing

#### Scenario: A Prometheus install needs no VictoriaMetrics knowledge
- **WHEN** an operator enables the bundle against Prometheus and Alertmanager
- **THEN** the ingest lane, the metrics tooling, the profile and the wiring all render and function, and the only VictoriaMetrics-specific value is the self-registration component, which stays off

### Requirement: The wiring component ships one claiming Pipeline, off by default
The bundle SHALL offer a wiring component rendering a `Pipeline` that claims the
bundle's own alert source and names the bundle's own profile, binding the
bundle's metrics toolset and `MCPConfig`. `pipelines.enabled` SHALL default to
`false`, and — unlike the Kubernetes bundle — NO values path SHALL force it on,
because no turnkey mode enables this bundle at all. The component SHALL render
only when the profile component renders, since a Pipeline with no profile has no
agent to run.

Exactly ONE route SHALL be offered. A metrics query server is read-only, so
there is no second posture to express.

The component SHALL render the ServiceAccount that route executes under, unless
the install names one, and SHALL bind it no Kubernetes RBAC by default.

Every reference the Pipeline makes to an object the bundle does not itself render
SHALL be a values-supplied name, omitted when unset. Channels SHALL be such a
list and SHALL default to empty; with none bound, the conversation dispatches
without waiting and its answer is readable from `status.runs[].result`. A ref to
a bundle component that is turned off SHALL be omitted rather than dangling.

Rendering alongside an install-declared Pipeline claiming the same source SHALL
be possible and SHALL NOT fail: sources are shareable. It SHALL be reported in
the post-install notes for what it is — one alert opening two conversations,
under two profiles, with two agents acting.

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
- **THEN** the rendered Pipeline carries that `channelRefs` entry; with the list
  empty the field is absent, not empty-valued

#### Scenario: A disabled component leaves no dangling reference
- **WHEN** wiring renders with the metrics component turned off
- **THEN** the Pipeline omits the toolset and MCPConfig refs entirely rather than
  naming objects nobody created

#### Scenario: The install also claims the source
- **WHEN** the bundle's wiring is active and an install-declared Pipeline also
  lists the bundle's alert source
- **THEN** both render, and the post-install notes state that each alert now
  opens two conversations
