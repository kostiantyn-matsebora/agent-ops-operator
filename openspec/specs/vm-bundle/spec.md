# vm-bundle

## Purpose

The VictoriaMetrics Helm subchart composition at `chart/charts/vm-bundle/`: packages the VM Alertmanager signal adapter with its webhook Service, ships `vmlogs`/`vmmetrics` MCPConfig CRs (with optionally deployed MCP server workloads) plus a `vm-observability` MCPToolset, and bundles no profile — tool wiring targets the operator's own Pipeline, since profiles carry no capabilities. Off by default and independent of demo mode.

## Requirements

### Requirement: The VM bundle ships as a self-gated subchart, off by default and independent of demo mode
A Helm subchart at `chart/charts/vm-bundle/` SHALL package the VictoriaMetrics experience as three components — the VM Alertmanager signal source and the `vmlogs`/`vmmetrics` MCP configurations. Every bundle template SHALL gate on `vm-bundle.enabled` (default `false`) alone; `global.demo.enabled` SHALL NOT enable this bundle, because its components require operator-supplied VictoriaMetrics endpoints that no demo cluster provides. Component flags (`alertmanager.enabled`, `mcp.vmlogs.enabled`, `mcp.vmmetrics.enabled`, all default `true`) SHALL independently control their objects within an active bundle.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, Service, SignalSource, or MCPConfig object from the bundle is rendered

#### Scenario: Demo mode does not enable the VM bundle
- **WHEN** the chart is installed with `global.demo.enabled=true` and default `vm-bundle` values
- **THEN** no VM bundle object is rendered

#### Scenario: Components toggle independently
- **WHEN** the bundle is enabled with `alertmanager.enabled=false`
- **THEN** only the enabled MCPConfig CRs render, with no adapter or Service

### Requirement: The alertmanager component packages the adapter with its webhook Service
When active, the `alertmanager` component SHALL render the `SignalAdapter` CR only (values-configured image, `port: 8080`, singleton) — **the Service and `LISTEN_ADDR` are appliance owned by the SignalAdapter reconciler via `spec.port`; the chart ships no connectivity** — plus the optional default `SignalSource` and the `Pipeline` claiming it (gated on `defaultSource.enabled` with a strictly required `defaultSource.profileRef`). The default SignalSource's `spec.adapter` SHALL render from the adapter's name value, so configuration can never drift from the implementation it targets. The pod-label selector contract remains pinned by an integration-test assertion.

#### Scenario: One flag exposes a working webhook URL
- **WHEN** the chart is installed with `vm-bundle.enabled=true` and a served source exists
- **THEN** `http://agentops-signal-<name>.<ns>.svc:8080/webhook/<source>` accepts Alertmanager webhooks — the Service comes from the reconciler, not the chart

#### Scenario: Default source requires an explicit profile
- **WHEN** `alertmanager.defaultSource.enabled=true` and `defaultSource.profileRef` is empty
- **THEN** rendering fails with a message naming the missing value rather than emitting an unwired SignalSource whose signals would drop

#### Scenario: Default source is wired by its rendered Pipeline
- **WHEN** the default source renders with a profileRef
- **THEN** a Pipeline claiming the SignalSource renders alongside it, so the source shows `Wired=True` and its signals route to the configured profile (and channels, when set)

#### Scenario: Source type follows the adapter name
- **WHEN** the bundle renders with `alertmanager.name: vm-alertmanager` and the default source enabled
- **THEN** the SignalSource carries `spec.adapter: vm-alertmanager` and the adapter serves it

### Requirement: MCP components ship vmlogs and vmmetrics as referencable MCPConfig CRs
When active, the `mcp.vmlogs` and `mcp.vmmetrics` components SHALL each render an `MCPConfig` CR (default names `vm-logs` / `vm-metrics`, values-overridable) whose single server entry uses the FIXED server key `victorialogs` / `victoriametrics` respectively (the key is the `mcp__<key>__*` tool namespace named by toolsets and SHALL NOT be values-configurable), `type: sse`, a values-configured URL, and values-passthrough `headers` supporting `valueFrom` secret references for authenticated endpoints (the manager reads no Secrets). An enabled MCP component with an empty URL SHALL fail rendering with a message naming the required value. The ONLY way to wire the bundle's tools into a wiring SHALL be the Pipeline tooling stanza (`mcpConfigs` refs + the bundle's `MCPToolset`) — there is no profile-side alternative, because AgentProfiles carry no capabilities.

#### Scenario: MCPConfigs render for Pipelines to bind
- **WHEN** the bundle is enabled with `mcp.vmlogs.url` and `mcp.vmmetrics.url` set, and the Pipeline routing the alerts binds both CRs in `mcpConfigs.refs` alongside a toolset granting `mcp__victorialogs__*`/`mcp__victoriametrics__*`
- **THEN** conversations from that Pipeline get working victorialogs and victoriametrics MCP tools

#### Scenario: Missing endpoint fails loudly
- **WHEN** the bundle is enabled with `mcp.vmlogs.enabled=true` and `mcp.vmlogs.url` empty
- **THEN** `helm template`/`install` fails naming `vm-bundle.mcp.vmlogs.url` instead of rendering an MCPConfig that points nowhere

#### Scenario: Authenticated endpoint without manager Secret reads
- **WHEN** `mcp.vmmetrics.headers` carries an `Authorization` header with a `secretKeyRef`
- **THEN** the rendered MCPConfig embeds the `valueFrom` reference and the credential is resolved only in the runtime pod, never read by the manager

### Requirement: MCP components can optionally deploy the MCP server workloads
Each MCP component SHALL support a `deploy` sub-block (default `enabled: false`): when enabled it renders the upstream MCP server image (values-configured) as a Deployment and Service (`agentops-mcp-<name>`) running in SSE mode against a required `deploy.backend` URL (the VictoriaLogs/VictoriaMetrics instance; empty fails the render naming the value). When the workload is deployed and the component's `url` is empty, the MCPConfig SHALL default to the deployed Service's SSE URL instead of failing; an explicit `url` still wins.

#### Scenario: Deployed MCP server wires itself
- **WHEN** the bundle renders with `mcp.vmlogs.deploy.enabled=true`, a backend URL, and no `mcp.vmlogs.url`
- **THEN** a Deployment and Service render and the MCPConfig's server URL is the deployed Service's SSE endpoint

#### Scenario: Deploy without a backend fails loudly
- **WHEN** `mcp.vmlogs.deploy.enabled=true` with an empty `deploy.backend`
- **THEN** rendering fails naming `vm-bundle.mcp.vmlogs.deploy.backend`

#### Scenario: Deployment stays off by default
- **WHEN** the bundle is enabled with default deploy values and explicit URLs
- **THEN** no MCP server Deployment or Service renders — endpoint-consumption mode is unchanged

### Requirement: The bundle ships a ready-made observability toolset
When any MCP component is active, the bundle SHALL render an `MCPToolset` (default name `vm-observability`, values-overridable) whose `tools` carry the tool namespaces of the enabled components only (`mcp__victorialogs__*` when vmlogs is on, `mcp__victoriametrics__*` when vmmetrics is on). Attaching the bundle's tools to a wiring SHALL thereby be a single Pipeline stanza — `mcpConfigs: {refs: [vm-logs, vm-metrics]}` plus `toolsets: {refs: [vm-observability]}`. The AgentProfile is not involved at all: it declares no capabilities, so there is no profile edit to avoid or to prefer.

#### Scenario: Toolset tracks enabled components
- **WHEN** the bundle renders with `mcp.vmlogs.enabled=true` and `mcp.vmmetrics.enabled=false`
- **THEN** `vm-observability` grants only `mcp__victorialogs__*`

#### Scenario: One pipeline stanza attaches the bundle's tools
- **WHEN** an operator adds the `mcpConfigs` + `toolsets` stanzas to the Pipeline routing the bundle's SignalSource
- **THEN** that pipeline's conversations can query VictoriaLogs/VictoriaMetrics via MCP, and the AgentProfile is untouched because it never carried tooling

### Requirement: The bundle ships no profile — tool wiring targets the operator's own Pipeline
The bundle SHALL NOT render an AgentProfile (user decision 2026-08-07: the alert-handling profile is operator-owned and typically already exists). `defaultSource.profileRef` is strictly required when the default source is enabled. The documentation SHALL present the Pipeline tooling stanza (`mcpConfigs: {refs: [vm-logs, vm-metrics]}` + `toolsets: {refs: [vm-observability]}`) as the one manual wiring step. There is no profile-editing alternative: capabilities live only on Pipelines. To grant these tools on EVERY route to a profile rather than one, the documentation SHALL direct the operator to that profile's capability-only Pipeline — its baseline.

#### Scenario: No profile objects from the bundle
- **WHEN** the bundle renders fully enabled
- **THEN** no `AgentProfile` object appears in the output

#### Scenario: Default source always requires an explicit profile
- **WHEN** `defaultSource.enabled=true` with an empty `profileRef`
- **THEN** rendering fails naming the value — there is no bundled profile to fall back to

#### Scenario: Granting the tools everywhere goes through the baseline
- **WHEN** an operator wants the VM tools on every route to their profile, not just the alert route
- **THEN** the documented step is to add the same stanza to that profile's capability-only Pipeline, since the profile itself can carry nothing

### Requirement: The registration component wires the in-cluster VMAlertmanager declaratively
When `alertmanager.registration.enabled=true` (default false) with a
required `registration.vmalertmanager: {name, namespace}`, the bundle SHALL:
set `kubernetesAccess: true` on the rendered SignalAdapter; render a Role
scoped to `vmalertmanagerconfigs.operator.victoriametrics.com`
(get/list/create/update/patch) plus a RoleBinding for the adapter's
deterministic ServiceAccount (`agentops-signal-<name>`) into the
VMAlertmanager's namespace; and put the `register` block (target plus the
optional routing knobs `matchers`, `groupWait`, `groupInterval`,
`repeatInterval`, `maxAlerts`, `sendResolved`) into the default source's opaque config — so one
flag yields an end-to-end path where the sender is configured by the
adapter, with no manual alertmanager edits. Rendering SHALL fail loudly when
`registration.enabled` is set without the target reference. Install notes
SHALL state whether registration is automatic or print the manual webhook
URL. The documentation SHALL note the vm-operator namespace-matcher caveat
(`VMAlertmanager.spec.disableNamespaceMatcher` for cluster-wide routing).

#### Scenario: One flag wires the sender
- **WHEN** the bundle renders with `registration.enabled=true` and a valid `vmalertmanager` reference alongside the default source
- **THEN** the SignalAdapter carries `kubernetesAccess: true`, the Role+RoleBinding land in the target namespace bound to `agentops-signal-<adapter>`, and the SignalSource config carries the `register` block

#### Scenario: Registration without a target fails at render time
- **WHEN** `registration.enabled=true` but `registration.vmalertmanager` is empty
- **THEN** rendering fails naming the missing value

#### Scenario: Disabled registration changes nothing
- **WHEN** the bundle renders with registration disabled
- **THEN** no RBAC objects render, `kubernetesAccess` is unset, and the source config carries no `register` block
