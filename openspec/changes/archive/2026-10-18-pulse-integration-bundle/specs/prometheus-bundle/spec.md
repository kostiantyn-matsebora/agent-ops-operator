## Purpose

Folded into this change alongside the Pulse bundle, for the same reason:
`pipelines.enabled` names one of the two CRD shapes a bundle's wiring can
take, even though the flag gates both — this rewrites it as `wiringEnabled`,
paired with `global.agentops.wiringMode`, across every bundle that ships one.

## MODIFIED Requirements

### Requirement: The wiring component ships one claiming Pipeline, off by default
The bundle SHALL offer a wiring component rendering a `Pipeline` that claims the bundle's own alert source and names the bundle's own profile, binding the bundle's metrics toolset and `MCPConfig`.

`wiringEnabled` SHALL default to `false`, and — unlike the Kubernetes bundle — NO values path SHALL force it on, because no turnkey mode enables this bundle at all.

The component SHALL render only when the profile component renders, since a Pipeline with no profile has no agent to run.

Exactly ONE route SHALL be offered. A metrics query server is read-only, so there is no second posture to express.

The component SHALL render the ServiceAccount that route executes under, unless the install names one, and SHALL bind it no Kubernetes RBAC by default.

Every reference the Pipeline makes to an object the bundle does not itself render SHALL be a values-supplied name, omitted when unset. Channels SHALL be such a list and SHALL default to empty.

With none bound, the conversation dispatches without waiting and its answer is readable from `status.runs[].result`.

A ref to a bundle component that is turned off SHALL be omitted rather than dangling.

Rendering alongside an install-declared Pipeline claiming the same source SHALL be possible and SHALL NOT fail: sources are shareable. It SHALL be reported in the post-install notes for what it is — one alert opening two conversations, under two profiles, with two agents acting.

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
- **THEN** the rendered Pipeline carries that `channelRefs` entry. With the list
  empty, the field is absent, not empty-valued

#### Scenario: A disabled component leaves no dangling reference
- **WHEN** wiring renders with the metrics component turned off
- **THEN** the Pipeline omits the toolset and MCPConfig refs entirely rather than
  naming objects nobody created

#### Scenario: The install also claims the source
- **WHEN** the bundle's wiring is active and an install-declared Pipeline also
  lists the bundle's alert source
- **THEN** both render, and the post-install notes state that each alert now
  opens two conversations

### Requirement: Each component is individually toggleable
Within an active bundle, `alertmanager.enabled`, `mcp.enabled`, `mcpServers.enabled`, `profile.enabled` and `wiringEnabled` SHALL independently control their component's objects, so partial enablement works and cross-component references stay values-resolvable.

`wiringEnabled` SHALL default `false` — the exception, because a route is the one component that spends money and acts on its own. `mcp` and `mcpServers` SHALL default off together, because both require an operator-supplied endpoint.

Wiring the bundle renders SHALL reference only objects the bundle itself renders, plus values-supplied names omitted when unset. A route spanning components the bundle cannot see SHALL be declared by the install, in the parent chart's `pipelines:`.

#### Scenario: Ingest-only bundle
- **WHEN** the bundle is enabled with the metrics, profile and wiring components
  off
- **THEN** only the SignalAdapter and the source render, and the install claims
  that source from its own wiring

#### Scenario: Tooling without the ingest lane
- **WHEN** the bundle is enabled with the ingest component off
- **THEN** the `MCPConfig` and toolset render for operators to bind from their
  own Pipelines
