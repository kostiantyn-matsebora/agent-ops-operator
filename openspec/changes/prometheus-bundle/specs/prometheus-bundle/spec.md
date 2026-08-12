## ADDED Requirements

### Requirement: The bundle ships as a self-gated subchart named for the protocol, not a vendor
A Helm subchart at `chart/charts/prometheus-bundle/` SHALL package the
Alertmanager alert-handling experience as five components — the Alertmanager
ingest lane, the Prometheus metrics MCP configuration, its deployable MCP server
workload, the alert-investigator profile, and the bundle's own wiring. Every
template SHALL gate on `prometheus-bundle.enabled` (default `false`) alone;
`global.demo.enabled` SHALL NOT enable this bundle, because its components
require operator-supplied metrics and Alertmanager endpoints that no demo cluster
provides.

The bundle SHALL be named for the payload format and query language it speaks,
not for one implementation of them. VictoriaMetrics SHALL be a supported backend
rather than the subject: the ingest path accepts the standard Alertmanager
webhook payload from any sender, and the metrics path speaks the Prometheus HTTP
query API, which VictoriaMetrics serves.

Because the values key is renamed, an install still carrying a `vm-bundle:` key
SHALL FAIL the render, naming the new key. Helm reports nothing when a values key
stops being read, so an unguarded rename would present as a bundle that installed
successfully and rendered nothing.

The bundle SHALL NOT render the agent's execution substrate. The `AgentRuntime`,
the runtime ServiceAccount, the LLM credential Secret and the runtime's RBAC are
the parent chart's.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, MCPConfig, MCPToolset, AgentProfile,
  Pipeline or workload from the bundle is rendered

#### Scenario: Demo mode does not enable this bundle
- **WHEN** the chart is installed with `global.demo.enabled=true` and default
  bundle values
- **THEN** no object from this bundle renders, because it consumes endpoints a
  demo cluster does not have

#### Scenario: The retired values key fails the render
- **WHEN** an install upgrades while still carrying a `vm-bundle:` values key
- **THEN** the render FAILS naming `prometheus-bundle`, rather than succeeding
  with a bundle that silently renders nothing

#### Scenario: A Prometheus install needs no VictoriaMetrics knowledge
- **WHEN** an operator enables the bundle against Prometheus and Alertmanager
- **THEN** the ingest lane, the metrics tooling, the profile and the wiring all
  render and function, and the only VictoriaMetrics-specific value is the
  self-registration component, which stays off

### Requirement: The ingest component packages the adapter with its webhook Service
When active, the `alertmanager` component SHALL render the `SignalAdapter` CR
(values-configured image, `port: 8080`, singleton) — the Service and
`LISTEN_ADDR` are owned by the SignalAdapter reconciler via `spec.port`, and the
chart ships no connectivity — plus, under `defaultSource.enabled`, a
`SignalSource` naming that adapter. The claim on that source SHALL belong to the
wiring component or to the install, never to this one, so an ingest lane can be
rendered for a route declared anywhere.

The adapter CR name is the ROUTING KEY that every `SignalSource` names in
`spec.adapter`. Its default SHALL be `alertmanager` — vendor-neutral, matching
the bundle — and SHALL remain values-configurable so an install migrating from
the previous default can restore it with one value rather than editing every
hand-written source. The default source's `spec.adapter` SHALL render from the
adapter's name value, so configuration can never drift from the implementation it
targets.

The pod-label selector contract SHALL remain pinned by an integration-test
assertion.

#### Scenario: One flag exposes a working webhook URL
- **WHEN** the bundle is enabled and a served source exists
- **THEN** `http://agentops-signal-<name>.<ns>.svc:8080/webhook/<source>` accepts
  Alertmanager webhooks, with the Service coming from the reconciler rather than
  the chart

#### Scenario: The source follows the adapter name
- **WHEN** the bundle renders with a values-overridden adapter name and the
  default source enabled
- **THEN** the SignalSource carries that same name in `spec.adapter`

#### Scenario: The ingest lane renders without a claim
- **WHEN** the ingest component is active and neither the wiring component nor
  the install claims its source
- **THEN** the adapter and the source render, and the source reports
  `Wired=False` and drops its alerts

### Requirement: Sender self-registration is a VictoriaMetrics-only sub-feature and says so
The `registration` sub-component SHALL keep its behavior: when enabled with a
required target reference it SHALL set `kubernetesAccess: true` on the rendered
SignalAdapter, render a Role scoped to
`vmalertmanagerconfigs.operator.victoriametrics.com` plus a RoleBinding for the
adapter's deterministic ServiceAccount into the target namespace, and place the
`register` block — target plus the optional routing knobs `matchers`,
`groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts`, `sendResolved` —
into the source's opaque config. Rendering SHALL fail loudly when registration is
enabled without the target reference. Registration failure SHALL never unserve
the source: the webhook stays live and the cause plus the manual step are carried
on the source's Ready condition.

This sub-feature SHALL be documented as working ONLY against VictoriaMetrics, and
as being impossible to generalize: it configures the sender by writing a
`VMAlertmanagerConfig`, and vanilla Alertmanager's configuration is a file or a
Secret, not a Kubernetes object an adapter can write. Presenting it without that
statement inside a vendor-neutral bundle would read as a feature merely awaiting
implementation for other senders.

#### Scenario: One flag wires a VictoriaMetrics sender
- **WHEN** the bundle renders with registration enabled and a valid target
  alongside the default source
- **THEN** the SignalAdapter carries `kubernetesAccess: true`, the Role and
  RoleBinding land in the target namespace bound to the adapter's SA, and the
  source's config carries the `register` block

#### Scenario: Registration without a target fails at render time
- **WHEN** registration is enabled with no target reference
- **THEN** the render fails naming the missing value

#### Scenario: A Prometheus install is told this does not apply to it
- **WHEN** an operator running vanilla Alertmanager reads the registration values
  or documentation
- **THEN** it states that registration is VictoriaMetrics-only because there is no
  sender-side object to write, and points at the printed receiver configuration
  instead

### Requirement: Vanilla Alertmanager is served by printed receiver configuration
Because no object exists for the adapter to write, the chart's post-install notes
SHALL print the exact Alertmanager `receivers:` entry pointing at the rendered
webhook URL for each rendered source. The printed configuration SHALL include the
bearer-credential form when the source carries a `credentialsSecretRef`, and
SHALL set `send_resolved: false`, because the adapter drops non-firing alerts —
a sender left at its default posts resolutions that are discarded, which presents
as an ingest fault from the sender's side.

#### Scenario: The notes print a usable receiver
- **WHEN** the bundle renders with a default source and registration off
- **THEN** the post-install notes print a `receivers:` entry with the Service
  webhook URL and `send_resolved: false`

#### Scenario: A credentialed source prints its auth form
- **WHEN** the rendered source carries a `credentialsSecretRef`
- **THEN** the printed receiver also carries the bearer credential form, so the
  webhook is not configured to fail authentication

### Requirement: One metrics MCP component, keyed for the query API it speaks
When active, the `mcp` component SHALL render an `MCPConfig` (values-overridable
name) whose single server entry uses the FIXED server key `prometheus` — the key
IS the `mcp__prometheus__*` tool namespace named by allowlists and SHALL NOT be
values-configurable, because a values rename would silently strip an agent's
tools — plus an `MCPToolset` granting that namespace. The transport and
values-passthrough `headers` SHALL support `valueFrom` secret references for
authenticated endpoints, resolved in the runtime pod, with the manager reading no
Secrets.

ONE key SHALL serve both backends. VictoriaMetrics answers the Prometheus HTTP
query API and reports a Prometheus version on `buildinfo` for clients that
version-gate, and MetricsQL is a PromQL superset, so a second vendor-keyed
namespace would be two names for one capability, chosen by a fact the operator
has already encoded in the endpoint URL.

The backend URL SHALL be values-supplied and SHALL NOT be derived from any
assumption about the backend's path layout: single-node VictoriaMetrics serves
`/api/v1`, cluster mode serves `/select/<accountID>/prometheus/api/v1`, and
Prometheus serves `/api/v1` under whatever external URL it was given. An enabled
component with neither a deployed server nor a URL SHALL fail the render naming
the required value, because an `MCPConfig` pointing nowhere costs agents their
tools silently.

The ONLY way to wire the bundle's tools into a route SHALL be the Pipeline
tooling stanza — `mcpConfigs` refs plus the toolset ref. There is no profile-side
alternative, because AgentProfiles carry no capabilities.

#### Scenario: The tools reach a conversation through wiring
- **WHEN** the metrics component renders and the Pipeline routing the alerts
  binds both the MCPConfig and the toolset
- **THEN** conversations from that Pipeline can query metrics through
  `mcp__prometheus__*`

#### Scenario: One key serves a VictoriaMetrics backend
- **WHEN** the component is pointed at a VictoriaMetrics query endpoint
- **THEN** the same `prometheus` server key and the same toolset apply, with no
  vendor-specific configuration beyond the URL

#### Scenario: Missing endpoint fails loudly
- **WHEN** the component is enabled with no deployed server and no URL
- **THEN** the render fails naming the required value rather than emitting an
  MCPConfig that points nowhere

#### Scenario: Authenticated endpoint without manager Secret reads
- **WHEN** the component's headers carry an `Authorization` entry with a
  `secretKeyRef`
- **THEN** the rendered MCPConfig embeds the `valueFrom` reference and the
  credential is resolved only in the runtime pod

### Requirement: The metrics MCP server workload is deployable under its own identity
The `mcpServers` component SHALL optionally deploy a Prometheus MCP server as a
Deployment and Service against a REQUIRED values-supplied backend URL, failing
the render when that URL is empty. When the workload is deployed and the `mcp`
component's URL is empty, the `MCPConfig` SHALL default onto the deployed
Service; an explicit URL SHALL still win. The two components SHALL flip together
by default so the config always has an endpoint to default onto.

Unlike the equivalent component in the Kubernetes bundle, both SHALL default OFF,
because there is no in-cluster endpoint to default onto — the backend is
operator-supplied by definition.

The server SHALL run under its OWN ServiceAccount and SHALL NOT be permitted to
run as the runtime ServiceAccount; setting them equal SHALL fail the render. It
SHALL require no Kubernetes RBAC, because it reads an HTTP query endpoint rather
than the Kubernetes API, and the bundle SHALL therefore render none for it.

#### Scenario: A deployed server wires itself
- **WHEN** the server component is enabled with a backend URL and the config's
  URL left empty
- **THEN** a Deployment and Service render and the MCPConfig's server URL is the
  deployed Service's endpoint

#### Scenario: Deploy without a backend fails loudly
- **WHEN** the server component is enabled with an empty backend
- **THEN** the render fails naming the required value

#### Scenario: The server never borrows the agent's identity
- **WHEN** the server's ServiceAccount name is set equal to the runtime
  ServiceAccount
- **THEN** the render fails, because collapsing the two identities removes the
  isolation the component exists to provide

### Requirement: The bundle ships an alert-investigator profile
When active, the `profile` component SHALL render exactly one object: an
alert-investigator `AgentProfile` (values-configurable name, `maxTurns`, no
repository, and NO capabilities — no `allowedTools`, no `mcp`). It SHALL render
no `AgentRuntime`, no ServiceAccount and no credential Secret; the profile
executes on the parent chart's runtime, and `runtimeRef` left empty SHALL emit no
reference and fall back to the runtime the parent guarantees.

Because the profile has NO repository, no agent definition file can be resolved
for it. The component SHALL therefore ship an inline role, so an alert does not
wake a personality-free agent whose only inputs are an allowlist and a payload.
The role SHALL direct the agent to read the alert, query the metric that fired
before concluding anything, state the likely cause with its evidence, and answer
briefly.

Shipping this profile is what makes the bundle's own wiring permissible: a
subchart may render a Pipeline only when that Pipeline renders with its own
profile.

#### Scenario: The profile component renders one object
- **WHEN** the bundle renders with the profile component on
- **THEN** the component's output is the `AgentProfile` alone, and no
  `AgentRuntime`, ServiceAccount or Secret carries a bundle label

#### Scenario: The profile stays free of capabilities
- **WHEN** the bundle renders with the metrics component active
- **THEN** the `MCPConfig` is referenced by whichever Pipeline routes the
  conversation, and the profile itself declares no `mcp` block and no tools

#### Scenario: The repo-less agent still has a role
- **WHEN** the bundle renders with defaults
- **THEN** the profile carries an inline role describing how to investigate an
  alert, which the runtime appends to its system prompt

### Requirement: The wiring component ships one claiming Pipeline, off by default
The bundle SHALL offer a wiring component rendering a `Pipeline` that claims the
bundle's own alert source and names the bundle's own profile, binding the
bundle's metrics toolset and `MCPConfig`. `pipelines.enabled` SHALL default to
`false`, and — unlike the Kubernetes bundle — NO values path SHALL force it on,
because no turnkey mode enables this bundle at all. The component SHALL render
only when the profile component renders, since a Pipeline with no profile has no
agent to run.

Exactly ONE route SHALL be offered. A metrics query server is read-only, so there
is no second posture to express and no derivation from the release's RBAC mode.

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

### Requirement: Each component is individually toggleable
Within an active bundle, `alertmanager.enabled`, `mcp.enabled`,
`mcpServers.enabled`, `profile.enabled` and `pipelines.enabled` SHALL
independently control their component's objects, so partial enablement works and
cross-component references stay values-resolvable.

`pipelines.enabled` SHALL default `false` — the exception, because a route is the
one component that spends money and acts on its own. `mcp` and `mcpServers` SHALL
default off together, because both require an operator-supplied endpoint.

Wiring the bundle renders SHALL reference only objects the bundle itself renders,
plus values-supplied names omitted when unset; a route spanning components the
bundle cannot see SHALL be declared by the install, in the parent chart's
`pipelines:`.

#### Scenario: Ingest-only bundle
- **WHEN** the bundle is enabled with the metrics, profile and wiring components
  off
- **THEN** only the SignalAdapter and the source render, and the install claims
  that source from its own wiring

#### Scenario: Tooling without the ingest lane
- **WHEN** the bundle is enabled with the ingest component off
- **THEN** the `MCPConfig` and toolset render for operators to bind from their
  own Pipelines
