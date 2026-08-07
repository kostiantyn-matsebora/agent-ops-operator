# vm-bundle

## Purpose

The VictoriaMetrics Helm subchart composition at `chart/charts/vm-bundle/`: packages the VM Alertmanager signal adapter with its webhook Service, ships `vmlogs`/`vmmetrics` MCPConfig CRs (with optionally deployed MCP server workloads), and bundles no profile — tool wiring targets an operator-owned profile. Off by default and independent of demo mode.

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
When active, the `alertmanager` component SHALL render: the `SignalAdapter` CR (`type: vmAlertmanagerWebhook`, values-configured image, singleton); a `Service` named `agentops-signal-<name>` selecting the reconciler's deterministic pod label `agentops.dev/signal-adapter: <name>` with a values-configured port and numeric `targetPort`; and an optional default `SignalSource` **plus the `Pipeline` claiming it** (wiring is pipeline-only), gated on `defaultSource.enabled` plus a configured, non-empty `defaultSource.profileRef` rendered onto the Pipeline (the bundle ships no profile — the reference targets an operator-owned profile; `defaultSource.channels` optionally binds mirroring channels). No `SignalAdapterSpec` schema changes and no reconciler changes SHALL be required; the pod-label selector contract SHALL be pinned by an integration-test assertion.

#### Scenario: One flag exposes a working webhook URL
- **WHEN** the chart is installed with `vm-bundle.enabled=true` and a served source exists
- **THEN** `http://agentops-signal-<name>.<ns>.svc:<port>/webhook/<source>` accepts Alertmanager webhooks without building images or applying extra manifests

#### Scenario: Default source requires an explicit profile
- **WHEN** `alertmanager.defaultSource.enabled=true` and `defaultSource.profileRef` is empty
- **THEN** rendering fails with a message naming the missing value rather than emitting an unwired SignalSource whose signals would drop

#### Scenario: Default source is wired by its rendered Pipeline
- **WHEN** the default source renders with a profileRef
- **THEN** a Pipeline claiming the SignalSource renders alongside it, so the source shows `Wired=True` and its signals route to the configured profile (and channels, when set)

### Requirement: MCP components ship vmlogs and vmmetrics as referencable MCPConfig CRs
When active, the `mcp.vmlogs` and `mcp.vmmetrics` components SHALL each render an `MCPConfig` CR (default names `vm-logs` / `vm-metrics`, values-overridable) whose single server entry uses the FIXED server key `victorialogs` / `victoriametrics` respectively (the key is the `mcp__<key>__*` tool namespace in profile allowlists and SHALL NOT be values-configurable), `type: sse`, a values-configured URL, and values-passthrough `headers` supporting `valueFrom` secret references for authenticated endpoints (the manager reads no Secrets). An enabled MCP component with an empty URL SHALL fail rendering with a message naming the required value. Wiring the MCPConfigs into a profile (via `mcp.configRefs` and `allowedTools`) is the operator's step and SHALL be documented with a complete example, including using the same profile as `defaultSource.profileRef` so the alert-handling agent can query logs and metrics.

#### Scenario: MCPConfigs render for profiles to reference
- **WHEN** the bundle is enabled with `mcp.vmlogs.url` and `mcp.vmmetrics.url` set and an AgentProfile lists both CRs in `mcp.configRefs` with the matching `mcp__victorialogs__*`/`mcp__victoriametrics__*` allowlist entries
- **THEN** conversations under that profile get working victorialogs and victoriametrics MCP tools

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

### Requirement: The bundle ships no profile — tool wiring targets the operator's own profile
The bundle SHALL NOT render an AgentProfile (user decision 2026-08-07, reverting an interim bundled-profile addition: the alert-handling profile is operator-owned and typically already exists). `defaultSource.profileRef` is strictly required when the default source is enabled, and the documentation SHALL present wiring the bundle's MCPConfigs into that profile (`mcp.configRefs` + the two tool-namespace allowlist entries) as the one manual step.

#### Scenario: No profile objects from the bundle
- **WHEN** the bundle renders fully enabled
- **THEN** no `AgentProfile` object appears in the output

#### Scenario: Default source always requires an explicit profile
- **WHEN** `defaultSource.enabled=true` with an empty `profileRef`
- **THEN** rendering fails naming the value — there is no bundled profile to fall back to
