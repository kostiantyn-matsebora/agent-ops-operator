# k8s-bundle

## Purpose

The Kubernetes agent Helm subchart composition at `chart/charts/k8s-bundle/`: packages the k8s events signal source, the `k8s-engineer` profile, the Kubernetes MCP tooling and its own wiring as individually toggleable components. It ships no execution SUBSTRATE: the `AgentRuntime`, the model credential, the home volume and the release-wide floor identity are the parent chart's (`agent-runtime-ownership`). It DOES render the ServiceAccount each route it ships runs as, because it is the only scope that knows what its own routes do. Self-gated and off by default, it is also what demo mode turns on — demo mode is an enablement path for the bundle's read-only defaults, not a distinct feature set.
## Requirements
### Requirement: The k8s bundle ships as a self-gated subchart, off by default and on in demo mode
A Helm subchart at `chart/charts/k8s-bundle/` SHALL package the Kubernetes agent experience as four components — events signal source, k8s-engineer profile, its MCP tooling, and its own wiring. Every bundle template SHALL gate on `enabled OR global.demo.enabled` (self-gating, not a Helm `condition:`), with `k8s-bundle.enabled` defaulting to `false`. The parent chart's demo toggle SHALL live at `global.demo.enabled` and there SHALL be no `chart/templates/demo.yaml` — demo mode means exactly "the bundle with its defaults", which includes read-only RBAC resolved by the parent AND the bundle's observing route. Explicit `k8s-bundle.*` values SHALL still apply when enabled via demo.

The bundle SHALL NOT render the agent's execution substrate. The `AgentRuntime`, the LLM credential Secret, the home volume and the release-wide floor identity are the parent chart's (`agent-runtime-ownership`), so a default install renders those and still renders nothing from this bundle. The ServiceAccount a route the bundle SHIPS executes under is the bundle's own, and is covered by the wiring component below.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, AgentProfile, Pipeline, MCP object, or RBAC object from the bundle is rendered — while the parent's `AgentRuntime` and runtime ServiceAccount do render, because they are not the bundle's

#### Scenario: Demo mode enables the bundle read-only
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** the bundle's components render, the parent's `AgentRuntime` named `default` carries the configured LLM credential env, the runtime SA holds read-only bindings, and the bundle's own observing Pipeline claims its events source — so work posted to that source is answered with no hand-written wiring; the source is what a caller names, never the Pipeline and never the profile

#### Scenario: Bundle without demo
- **WHEN** the chart is installed with `k8s-bundle.enabled=true` and `global.demo.enabled=false`
- **THEN** the same bundle components render — demo mode is an enablement path, not a distinct feature set — and the runtime SA holds no bindings unless `global.agentops.runtime.rbacMode` is set, and NO Pipeline renders unless the wiring flag is set

### Requirement: Each component is individually toggleable
Within an active bundle, `eventsAdapter.enabled`, `profile.enabled`, `mcp.enabled`, and `pipelines.enabled` SHALL independently control their component's objects. The first three SHALL default `true`, and the `mcpServers` sub-component that deploys the MCP server workload SHALL default `true` alongside `mcp`: the two flip together so the config's URL always has an endpoint to default onto, which is the only reason the MCP component was previously off. The endpoint guard SHALL remain and SHALL still fail loudly for `mcp.enabled` with no server workload and no `url`.

`pipelines.enabled` SHALL default `false` — the exception among the four, because a route is the one component of this bundle that spends money and touches the cluster on its own. Demo mode SHALL force it on; nothing else SHALL.

The bundle SHALL expose no runtime or runtime-RBAC component; those toggles are the parent's `runtime.enabled` and `global.agentops.runtime.rbacMode`.

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

### Requirement: The events component packages the adapter with its access
When active, the `eventsAdapter` component SHALL render: the `SignalAdapter` CR (default name `k8s-events` — the routing key SignalSources select with `spec.adapter` — values-configured image, `kubernetesAccess: true`, singleton); RBAC granting `events` `get`/`list`/`watch` AND `pods`/`replicasets` `list`/`watch` to the adapter's deterministic ServiceAccount `agentops-signal-<name>` (ClusterRole by default, namespaced Role via `rbac.clusterWide: false`, none via `rbac.create: false`); and, when `source.create` is on, a `SignalSource` naming that adapter with `severities` defaulting to `["Warning"]` and values-configurable `namespaces` and `grouping`. The claim on that source belongs to the wiring component or to the install — never to this one, so an events lane can be rendered for a route declared anywhere. The manager SHALL NOT create or require any RBAC verbs on roles or rolebindings.

The pods/replicasets grant is read-only and exists because the adapter resolves workload identity through owner references and re-checks liveness before emitting. Where the events grant is namespaced, the pods/replicasets grant SHALL be namespaced identically — the adapter never reads more broadly than it watches.

The rendered source's default `grouping.signatureLabels` SHALL be `["namespace", "workload"]`, and its default `rules` SHALL be calibrated against both failure modes at once: they SHALL NOT open a conversation for ordinary rollout churn, and they SHALL NOT lose an actionable incident. Specifically the shipped defaults SHALL drop only pure-bookkeeping reasons whose underlying problem another undropped reason still reports, SHALL assign `for: 0` to reasons describing a completed event and to node-level conditions, SHALL assign longer dwells with breadth escalation to the known-flappy reasons, and SHALL end in a catch-all dwell so unanticipated reasons are verified rather than discarded.

#### Scenario: A healthy rollout produces no conversation
- **WHEN** the bundle renders with default values and a ten-replica Deployment rolls out normally, emitting probe and scheduling warnings on pods that then become Ready or terminate
- **THEN** no conversation is created

#### Scenario: A broken rollout produces exactly one conversation
- **WHEN** the same Deployment is rolled out with an unpullable image
- **THEN** exactly one conversation is created for the workload, carrying every contributing reason with its occurrence count

#### Scenario: The events lane renders without a claim
- **WHEN** the events component is active and neither the wiring component nor the install claims its source
- **THEN** the adapter, its RBAC and the source render, and the source reports `Wired=False` and drops its signals

#### Scenario: One values flag yields flowing events
- **WHEN** the chart is installed with `global.demo.enabled=true` and the LLM credential Secret exists
- **THEN** Warning events in the cluster produce conversations executed by the k8s-engineer profile without building images or applying extra manifests, because demo mode turns the wiring component on alongside this one

#### Scenario: The rendered source is always claimed
- **WHEN** the events component renders a SignalSource while the wiring component is active
- **THEN** that component's Pipeline claims it, so signals route instead of dropping with `Wired=False` — the claim comes from the wiring component or the install, never from this one, which is what lets an events lane be rendered for a route declared anywhere

#### Scenario: Event-driven conversations are equipped
- **WHEN** an event routes through whichever Pipeline claims the source
- **THEN** the resulting work unit carries a non-empty allowlist, because that Pipeline declares its own toolsets

#### Scenario: Namespace-scoped events RBAC
- **WHEN** `eventsAdapter.rbac.clusterWide=false`
- **THEN** only a namespaced Role/RoleBinding renders, covering events, pods and replicasets alike, and the adapter can watch only in the release namespace

#### Scenario: A default install groups by workload
- **WHEN** the bundle renders with default values and a ten-replica Deployment crash-loops through several rollouts
- **THEN** the rendered source's `signatureLabels` are `["namespace", "workload"]` and one conversation covers the workload rather than one per pod

#### Scenario: Default rules ship, not an empty filter
- **WHEN** the bundle renders with default values
- **THEN** the rendered source carries a non-empty `rules` list, so rollout churn is suppressed without the installer writing any configuration

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render exactly one object: the `k8s-engineer` `AgentProfile` (values-configurable name, `maxTurns`, no repository, and **no capabilities** — no `allowedTools`, no `mcp`). It SHALL render no `AgentRuntime`, no ServiceAccount, and no credential Secret; the profile executes on the parent chart's runtime.

Because the profile has NO repository, no agent definition file can be resolved for it. The component SHALL therefore support an inline role (`systemPrompt`) and ship a sensible default, so the shipped agent is not personality-free: a conversation started by a cluster event would otherwise arrive with no instructions at all.

`profile.runtimeRef` SHALL remain, naming a runtime other than the parent's `default` — a higher-trust or different-vendor runtime the operator applied. Left empty, the profile emits no `runtimeRef` and falls back to `default`, which the parent guarantees exists whenever `runtime.enabled` is true.

#### Scenario: Profile executes under the release's runtime SA
- **WHEN** the bundle renders with defaults and a task reaches `k8s-engineer`
- **THEN** the conversation's runtime pod runs under `global.agentops.runtime.serviceAccountName`, so the agent's in-cluster power is exactly what `global.agentops.runtime.rbacMode` granted

#### Scenario: The profile component renders one object
- **WHEN** the bundle renders with `profile.enabled=true`
- **THEN** the component's output is the `AgentProfile` alone, and no `AgentRuntime`, ServiceAccount, or Secret carries a bundle label

#### Scenario: Pointing at a different runtime
- **WHEN** `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the `AgentProfile` renders with that `runtimeRef` and the parent's `default` runtime is left unused by this profile

#### Scenario: Fallback needs no wiring
- **WHEN** `profile.runtimeRef` is empty and `runtime.enabled` is true
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

### Requirement: Demo values migrate to bundle paths
The pre-bundle demo values SHALL move: `demo.enabled` → `global.demo.enabled`, `demo.readOnlyRbac` → `global.agentops.runtime.rbacMode` (true ≙ `readonly`). The runtime-shaped demo values SHALL land in the parent's `runtime:` block rather than in this bundle: `demo.runtimeImage` → `runtime.image`, `demo.credentialsSecret.*` → `runtime.credentialsSecret.*`, inherited `persistence` → automatic, inherited `runtimeIdleTtlMinutes` → the manager default with `runtime.idleTtlMinutes` as an override.

The chart major version SHALL be bumped and the README SHALL carry a migration table covering BOTH hops, so an operator who moved a value into `k8s-bundle.profile.runtime.*` in 2.x can find where it went. That table SHALL lead with the two upgrade-visible effects that are not value renames: the runtime ServiceAccount changes name (bundle-named bindings are replaced by global-named ones), and an install that enabled the bundle without configuring MCP gains an MCP server workload.

Upgrading SHALL preserve semantics: the `AgentRuntime` named `default` re-renders equivalently from the parent, so existing conversations keep resolving their runtime.

#### Scenario: Upgraded demo release keeps working
- **WHEN** a release running chart 3.x with the bundle enabled upgrades and adopts the new values paths
- **THEN** the agent flow works unchanged — now reached by posting a `kind: task` signal to the bundle's events source, which the install's Pipeline claims — the `default` runtime re-renders from the parent, and the bundle-named ServiceAccount and its bindings are removed by the upgrade

#### Scenario: Both migration hops are findable
- **WHEN** an operator looks up a value they set at `k8s-bundle.profile.runtime.*`
- **THEN** the migration table names its 4.0 location, rather than only documenting the 1.x → 2.x hop

### Requirement: The events component exposes maintenance windows as values
The bundle SHALL expose the events source's time intervals and mute references as chart values, so a recurring maintenance window is release configuration rather than a hand-edited CR that the next upgrade overwrites.

The shipped example SHALL name an IANA location rather than relying on the UTC default, because the value most likely to be copied unchanged is the one that must not be wrong.

#### Scenario: A maintenance window survives an upgrade
- **WHEN** an operator declares a nightly window in the bundle's values and upgrades the release
- **THEN** the rendered SignalSource carries the window, unchanged by the upgrade

#### Scenario: No window is configured by default
- **WHEN** the bundle renders with default values
- **THEN** the source declares no time intervals and nothing is muted

### Requirement: The wiring component ships at most one claiming Pipeline, off by default
The bundle SHALL offer a fourth component — its own wiring — rendering a
`Pipeline` that claims the bundle's `cluster-events` source and names the
bundle's `k8s-engineer` profile. `pipelines.enabled` SHALL default to `false`,
and SHALL be forced on by `global.demo.enabled` exactly as every other bundle
template is self-gated. The component SHALL render only when the profile
component renders, since a Pipeline with no profile has no agent to run.

The component SHALL offer TWO routes, differing in one binding:

- an OBSERVING route binding the built-in observation toolset, the bundle's read
  toolset and the bundle's `MCPConfig` — an agent that reads the cluster and
  changes nothing;
- an ACTING route binding the same plus the bundle's mutating toolset.

Which route renders SHALL DERIVE from `global.agentops.runtime.rbacMode`, the
same value the MCP server's read-only flag, the server SA's RBAC mode and the
mutating toolset already follow: `full` renders the acting route, and every
other mode — `readonly`, `none`, and unset — renders the observing route.
Explicit per-route values SHALL win in both directions.

Rendering both routes SHALL be possible and SHALL NOT fail: two Ready Pipelines
claiming one source is a supported shape. It SHALL be documented for what it
is — one event opening two conversations, under two profiles, with two agents
acting.

The component SHALL render the ServiceAccount each route it ships executes
under, unless the install names one on that route, and SHALL bind those accounts
no Kubernetes RBAC by default. A bundle is the only scope that knows what its own
routes do, and the parent restating it would be one fact in two places.

Holding no RBAC is the correct default rather than an omission: the runtime image
ships no `kubectl`, so an agent reaches the cluster ONLY through the MCP server,
which has its own account carrying the actual grant. What a per-route account
buys is IDENTITY — each route is a distinct subject, a later grant lands on ONE
route instead of every agent in the install, and a mistaken grant on the parent's
floor account cannot reach a bundle route.

Every reference the Pipeline makes to an object the bundle does not itself
render SHALL be a values-supplied name, omitted when unset. Channels SHALL be
such a list and SHALL default to empty; with none bound, the conversation
dispatches without waiting and its answer is readable from
`status.runs[].result`. A ref to a bundle component that is turned off SHALL be
omitted rather than dangling, so a route with the MCP component disabled renders
without cluster tooling instead of naming an MCPConfig nobody created.

#### Scenario: Default install renders no wiring
- **WHEN** the bundle is enabled with `k8s-bundle.enabled=true` and defaults
- **THEN** no `Pipeline` renders, the source reports `Wired=False`, and the
  install's own `pipelines:` remain the only routes

#### Scenario: Demo mode renders the observing route
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing
  else
- **THEN** exactly one `Pipeline` renders, claiming `cluster-events` with the
  read toolset and the `MCPConfig` and WITHOUT the mutating toolset
- **AND** an admitted event opens a conversation with no further configuration

#### Scenario: A write-capable release renders the acting route
- **WHEN** wiring is active and `global.agentops.runtime.rbacMode` is `full`
- **THEN** the acting route renders instead of the observing one, binding the
  mutating toolset that the write-mode server registers
- **AND** exactly one Pipeline claims the source

#### Scenario: An explicit route value overrides the derivation
- **WHEN** an operator sets the acting route's enable value explicitly under a
  `readonly` release, or disables it explicitly under `full`
- **THEN** the explicit value decides, in both directions

#### Scenario: Both routes are asked for
- **WHEN** an operator enables both routes explicitly
- **THEN** both Pipelines render and one admitted event opens two conversations,
  and the render does not fail

#### Scenario: Wiring is declined while the bundle stays on
- **WHEN** `pipelines.enabled=false` is set under `global.demo.enabled=true`
- **THEN** no Pipeline renders and every other bundle component is unaffected

#### Scenario: A channel is named
- **WHEN** `pipelines.channels` names an existing Channel
- **THEN** the rendered Pipeline carries that `channelRefs` entry; with the list
  empty the field is absent, not empty-valued

#### Scenario: The install also claims the source
- **WHEN** bundle wiring is active and an install-declared Pipeline also lists
  the bundle's source
- **THEN** both render, and the chart's post-install notes state that each event
  now opens two conversations

### Requirement: A shipped route claims the console so it can originate

Where the bundle renders a route at all, that route SHALL claim the console's
signal source and bind the console as a channel, so an install that deploys the
console can start a conversation in it without further wiring.

Both SHALL be values-supplied NAMES read from `global.` — the only parent scope a
subchart can read — for objects the bundle does not itself render, and both SHALL
be omitted when the parent names none. The channel SHALL be merged with any
channels the operator named rather than replacing them.

The claim SHALL ride the route the bundle already ships rather than adding a
second one: two routes claiming the console source would make every unaddressed
console message ambiguous, since a bare chat message with more than one claimant
is refused.

#### Scenario: Turnkey install can be used from the console

- **WHEN** the chart is installed with its turnkey flag and the console enabled
- **THEN** the rendered route claims the console's signal source and binds the
  console channel, and the console's composer is available

#### Scenario: The console is not deployed

- **WHEN** the console is disabled and the parent names no console source or
  channel
- **THEN** the route claims only the bundle's own source and binds no console
  channel

#### Scenario: An operator named their own channel

- **WHEN** the operator names a channel for the route and the console is enabled
- **THEN** both are bound, and neither replaces the other

#### Scenario: One route, not two

- **WHEN** the console source is claimed
- **THEN** it is claimed by the route the bundle already renders, so an
  unaddressed console message still has exactly one claimant

