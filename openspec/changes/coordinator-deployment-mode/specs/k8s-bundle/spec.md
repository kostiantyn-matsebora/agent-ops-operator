## Purpose

The Kubernetes agent Helm subchart composition at
`chart/charts/kubernetes/`. It packages the k8s events signal source, the
`k8s-engineer` profile, the Kubernetes MCP tooling and its own wiring as
individually toggleable components.

It ships no execution SUBSTRATE. The `AgentRuntime`, the model credential,
the context volume and the release-wide floor identity are the parent
chart's (`agent-runtime-ownership`).

It DOES render the ServiceAccount each route it ships runs as, because it
is the only scope that knows what its own routes do.

Self-gated and off by default, it is also what demo mode turns on — demo
mode is an enablement path for the bundle's read-only defaults, not a
distinct feature set.

## MODIFIED Requirements

### Requirement: The wiring component ships its routes as stated settings, off by default
Which route the bundle ships SHALL be a STATED SETTING, never a consequence
derived from a release-wide permission mode.

The derivation moved three things at once — the MCP server's read-only flag,
that server's RBAC width, and which of the two routes rendered — from one
value whose name mentioned none of them.

Each was individually overridable, so an operator reading their values could
not tell which of the three was in force.

They SHALL still be able to move together, stated as such. What is refused is a
setting whose NAME describes none of what it changes.

The bundle SHALL continue to render ONE identity per route it ships, and those
identities SHALL continue to hold no Kubernetes RBAC of their own: an agent
reaches the cluster through the MCP server, which carries the grant.

**AN ELEVATED ROUTE IS THE BUNDLE'S TO DECLARE.** The bundle is the only scope
that knows what its own routes do, so a route needing more than the MCP path
gives it SHALL get that from the bundle rather than from a release-wide preset.

Under `global.agentops.wiringMode: coordinator`, the bundle SHALL render the
SAME two route identities as standalone `AgentCapability` objects rather than
inline Pipelines.

- `k8s-observe` is the observing capability, at the privilege level the
  observing route uses today.
- `k8s-operate` is the acting capability, at the level the acting route uses
  today.
- The two SHALL NEVER be merged into one.

Coordinator mode renders the observing `AgentCapability` when its wiring is
enabled.

It renders the acting one too when `pipelines.admin.enabled` is set, exactly
as the per-route flags decide under `pipelines` mode.

Demo mode forces on only the observing one. The bundle SHALL NOT render its
own `Pipeline` in this mode.

This branch SHALL follow the four conditions `wiring-mode` restates from the
parent chart's bundle-wiring rules for a bundle's own routes:

- gated by the same explicit flag
- every foreign reference a values-supplied name, omitted when unset
- the `AgentCapability` rendered only alongside its own profile
- the flag defaulting off, with demo mode forcing on only the
  least-privileged route

#### Scenario: Default install renders no wiring
- **WHEN** the bundle is enabled with defaults
- **THEN** no `Pipeline` renders, the source reports `Wired=False`, and the install's own `pipelines:` remain the only routes

#### Scenario: Demo mode renders the observing route
- **WHEN** the chart is installed with `global.demo.enabled=true` and `global.agentops.wiringMode: pipelines` (demo mode otherwise selects coordinator posture, below)
- **THEN** exactly one `Pipeline` renders, claiming `cluster-events` with the read toolset and the `MCPConfig` and WITHOUT the mutating toolset
- **AND** an admitted event opens a conversation with no further configuration

#### Scenario: The acting route is chosen, not inferred
- **WHEN** an install wants the bundle's acting route
- **THEN** it sets `pipelines.admin.enabled: true` directly, and no release-wide permission value can select it instead

#### Scenario: Both routes are asked for
- **WHEN** an operator enables both routes explicitly
- **THEN** both Pipelines render and one admitted event opens two conversations, and the render does not fail

#### Scenario: Wiring is declined while the bundle stays on
- **WHEN** wiring is disabled under `global.demo.enabled=true`
- **THEN** no Pipeline renders and every other bundle component is unaffected

#### Scenario: A channel is named
- **WHEN** the wiring names an existing Channel
- **THEN** the rendered Pipeline carries that `channelRefs` entry — with the list empty the field is absent, not empty-valued

#### Scenario: The install also claims the source
- **WHEN** bundle wiring is active and an install-declared Pipeline also lists the bundle's source
- **THEN** both render, and the chart's post-install notes state that each event now opens two conversations

#### Scenario: Route identities still hold nothing
- **WHEN** the bundle ships its routes
- **THEN** each runs as its own account, and those accounts carry no Kubernetes RBAC

#### Scenario: Coordinator mode renders a capability, not a Pipeline

- **WHEN** `global.agentops.wiringMode: coordinator` is set and the bundle's
  wiring is enabled
- **THEN** the bundle renders the `k8s-observe` `AgentCapability` at the
  observing (read-only) privilege level, plus `k8s-operate` (the acting one) only when its
  own flag (`pipelines.admin.enabled`) is enabled, and renders no `Pipeline` of its own

#### Scenario: Coordinator mode's capability is claimed by the chart Coordinator

- **WHEN** coordinator mode is set and the bundle's wiring is enabled
- **THEN** the chart-rendered Coordinator's `agents[]` lists this bundle's
  `AgentCapability`, and its `signalSourceRefs` claims the bundle's events
  source — so the source is not left `Wired=False`

#### Scenario: Demo mode under coordinator posture still forces the safe route

- **WHEN** `global.demo.enabled=true` and `global.agentops.wiringMode:
  coordinator` are both set
- **THEN** demo mode forces on only the observing capability, never the
  acting one, matching the least-privileged rule that already governs demo
  mode under `pipelines` posture
