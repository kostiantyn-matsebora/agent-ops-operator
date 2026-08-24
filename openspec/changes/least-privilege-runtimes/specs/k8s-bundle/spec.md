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

### Requirement: Each component is individually toggleable
Within an active bundle, `eventsAdapter.enabled`, `profile.enabled`, `mcp.enabled`, and `pipelines.enabled` SHALL independently control their component's objects. The first three SHALL default `true`, and the `mcpServers` sub-component that deploys the MCP server workload SHALL default `true` alongside `mcp`: the two flip together so the config's URL always has an endpoint to default onto, which is the only reason the MCP component was previously off. The endpoint guard SHALL remain and SHALL still fail loudly for `mcp.enabled` with no server workload and no `url`.

`pipelines.enabled` SHALL default `false` — the exception among the four, because a route is the one component of this bundle that spends money and touches the cluster on its own. Demo mode SHALL force it on; nothing else SHALL.

The bundle SHALL expose no runtime-defaults or floor-account component; those are the parent's `global.agentops.runtimeDefaults` and `runtimes:`. It SHALL expose ONE stated setting for whether agents on its lane may CHANGE the cluster, which moves the server's read-only flag, that server's RBAC, the mutating toolset and which route ships — each still individually overridable, and none derived from a release-wide value.

Cross-component references SHALL be values-resolvable so partial enablement works. Wiring the bundle renders SHALL reference only objects the bundle itself renders, plus values-supplied names omitted when unset; a route that would span components the bundle cannot see SHALL be declared by the install, in the parent chart's `pipelines:`.

#### Scenario: Events-only bundle
- **WHEN** the bundle is enabled with `profile.enabled=false`, `mcp.enabled=false` and wiring off
- **THEN** only the SignalAdapter, its RBAC, and the SignalSource render, and the install claims that source from its own wiring

#### Scenario: Wiring without a profile
- **WHEN** wiring is enabled with `profile.enabled=false`
- **THEN** no Pipeline renders, because a Pipeline with no profile has no agent to run

#### Scenario: Profile-only bundle
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the profile renders and the agent is usable through any Pipeline naming it, executing on the parent's runtime

#### Scenario: A Kubernetes bundle can see Kubernetes by default
- **WHEN** the bundle is enabled with defaults
- **THEN** the `MCPConfig`, the read toolset, and the MCP server workload all render, so the install does not look complete while lacking the cluster access path the project prefers

#### Scenario: MCP tooling without the events lane
- **WHEN** the bundle is enabled with `eventsAdapter.enabled=false`
- **THEN** the `MCPConfig` and toolsets render for operators to bind from their own Pipelines

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render exactly one object: the `k8s-engineer` `AgentProfile` (values-configurable name, `maxTurns`, no repository, and **no capabilities** — no `allowedTools`, no `mcp`). It SHALL render no `AgentRuntime`, no ServiceAccount, and no credential Secret; the profile executes on the parent chart's runtime.

Because the profile has NO repository, no agent definition file can be resolved for it. The component SHALL therefore support an inline role (`systemPrompt`) and ship a sensible default, so the shipped agent is not personality-free: a conversation started by a cluster event would otherwise arrive with no instructions at all.

`profile.runtimeRef` SHALL remain, naming a runtime other than `default` — a different-vendor runtime the install declared. Left empty, the profile emits no `runtimeRef` and falls back to `default`, whose existence the render-time default-runtime guard is what guarantees.

#### Scenario: Profile executes under the release's runtime SA
- **WHEN** the bundle renders with defaults and a task reaches `k8s-engineer`
- **THEN** the conversation's runtime pod runs under the account its route names, or the release's default where it names none, and holds exactly what that account was granted

#### Scenario: The profile component renders one object
- **WHEN** the bundle renders with `profile.enabled=true`
- **THEN** the component's output is the `AgentProfile` alone, and no `AgentRuntime`, ServiceAccount, or Secret carries a bundle label

#### Scenario: Pointing at a different runtime
- **WHEN** `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the `AgentProfile` renders with that `runtimeRef` and the parent's `default` runtime is left unused by this profile

#### Scenario: Fallback needs no wiring
- **WHEN** `profile.runtimeRef` is empty and a runtime answers to `default`
- **THEN** the profile emits no `runtimeRef` and resolves the parent's `default` runtime

#### Scenario: The profile stays free of capabilities
- **WHEN** the bundle renders with the MCP component active
- **THEN** the `MCPConfig` is referenced by whichever Pipeline routes the conversation — the bundle's own or the install's — and the AgentProfile itself declares no `mcp` block and no tools, so profiles stay reusable across differently-tooled routes

#### Scenario: The demo reaches a Pipeline through the source it claims
- **WHEN** a `kind: task` signal is posted to a source claimed by the bundle's own wiring or by the install's Pipeline
- **THEN** the work unit carries that Pipeline's tools and the rendered AgentProfile declares none — the route is the one that claimed the source, and it is reached by posting to the source, never by naming the Pipeline or the profile

#### Scenario: An observe-only agent
- **WHEN** the claiming Pipeline binds the observation toolset without the shell or mutating ones
- **THEN** the agent reads the cluster but changes nothing, because the allowlist is the whole grant

#### Scenario: The repo-less agent still has a role
- **WHEN** the bundle renders with defaults
- **THEN** the AgentProfile carries an inline role describing the agent's job and how to act on a cluster, which the runtime appends to its system prompt

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
