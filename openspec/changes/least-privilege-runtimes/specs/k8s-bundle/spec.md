## MODIFIED Requirements

### Requirement: The k8s bundle ships as a self-gated subchart, off by default and on in demo mode
The bundle SHALL be named for the system it integrates — `kubernetes` — and SHALL
NOT carry a `-bundle` suffix, matching `prometheus`, `telegram`, `home-assistant`
and the vendor runtimes.

`k8s` alone is not descriptive, and the abbreviation collides in READING with the
`k8s-ops` and `k8s-observe` PIPELINE names an install declares — the same string
on two different kinds of object.

It SHALL remain self-gated and off by default, and demo mode SHALL still turn it
on. Renaming changes the key an operator sets, not when the bundle renders.

**THE RETIRED KEY SHALL FAIL THE RENDER**, naming the replacement. Helm reports
no unread values key, so a values file left on the old spelling would be silently
ignored and the bundle simply would not render — indistinguishable from an
operator who meant to leave it off.

#### Scenario: The bundle is named for what it integrates
- **WHEN** an install enables the Kubernetes bundle
- **THEN** it does so under a key naming Kubernetes, with no suffix

#### Scenario: The retired key is refused, not ignored
- **WHEN** a values file supplies the retired suffixed key
- **THEN** the render fails naming that key and its replacement

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, AgentProfile, Pipeline, MCP object, or RBAC object from the bundle is rendered — while a runtime answering to the default name still renders, because it is not the bundle's

#### Scenario: Demo mode enables the bundle read-only
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** the bundle's components render, a runtime named `default` carries the configured LLM credential env, and the bundle's own observing Pipeline claims its events source — so work posted to that source is answered with no hand-written wiring; the source is what a caller names, never the Pipeline and never the profile

#### Scenario: Bundle without demo
- **WHEN** the chart is installed with the bundle enabled and `global.demo.enabled=false`
- **THEN** the same bundle components render — demo mode is an enablement path, not a distinct feature set — and NO Pipeline renders unless the wiring flag is set

## REMOVED Requirements

### Requirement: The wiring component ships at most one claiming Pipeline, off by default
**Reason**: Replaced. Half its scenarios describe a DERIVATION from a
release-wide permission mode — the acting route rendering instead of the
observing one when that mode was widened, and an explicit value overriding the
derivation in both directions. The mode is removed, so those describe a
mechanism that no longer exists.

The properties that were not about the derivation — off by default, both routes
renderable, wiring declinable, a channel nameable, and the shared-source
fan-out reported rather than refused — are carried into the replacement.

## ADDED Requirements

### Requirement: The wiring component ships its routes as stated settings, off by default
Which route the bundle ships SHALL be a STATED SETTING, never a consequence
derived from a release-wide permission mode.

The derivation moved three things at once — the MCP server's read-only flag, that
server's RBAC width, and which of the two routes rendered — from one value whose
name mentioned none of them. Each was individually overridable, so an operator
reading their values could not tell which of the three was in force.

They SHALL still be able to move together, stated as such. What is refused is a
setting whose NAME describes none of what it changes.

The bundle SHALL continue to render ONE identity per route it ships, and those
identities SHALL continue to hold no Kubernetes RBAC of their own: an agent
reaches the cluster through the MCP server, which carries the grant.

**AN ELEVATED ROUTE IS THE BUNDLE'S TO DECLARE.** The bundle is the only scope
that knows what its own routes do, so a route needing more than the MCP path
gives it SHALL get that from the bundle rather than from a release-wide preset.

#### Scenario: Default install renders no wiring
- **WHEN** the bundle is enabled with defaults
- **THEN** no `Pipeline` renders, the source reports `Wired=False`, and the install's own `pipelines:` remain the only routes

#### Scenario: Demo mode renders the observing route
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** exactly one `Pipeline` renders, claiming `cluster-events` with the read toolset and the `MCPConfig` and WITHOUT the mutating toolset
- **AND** an admitted event opens a conversation with no further configuration

#### Scenario: The acting route is chosen, not inferred
- **WHEN** an install wants the bundle's acting route
- **THEN** it enables that route directly, and no release-wide permission value can select it instead

#### Scenario: Both routes are asked for
- **WHEN** an operator enables both routes explicitly
- **THEN** both Pipelines render and one admitted event opens two conversations, and the render does not fail

#### Scenario: Wiring is declined while the bundle stays on
- **WHEN** wiring is disabled under `global.demo.enabled=true`
- **THEN** no Pipeline renders and every other bundle component is unaffected

#### Scenario: A channel is named
- **WHEN** the wiring names an existing Channel
- **THEN** the rendered Pipeline carries that `channelRefs` entry; with the list empty the field is absent, not empty-valued

#### Scenario: The install also claims the source
- **WHEN** bundle wiring is active and an install-declared Pipeline also lists the bundle's source
- **THEN** both render, and the chart's post-install notes state that each event now opens two conversations

#### Scenario: Route identities still hold nothing
- **WHEN** the bundle ships its routes
- **THEN** each runs as its own account, and those accounts carry no Kubernetes RBAC
