## MODIFIED Requirements

### Requirement: RBAC is read-only by default with an explicit full mode
When active, the `rbac` component SHALL bind roles to the profile's runtime ServiceAccount according to `rbac.mode`: `readonly` (default) binds the built-in `view` ClusterRole plus a bundle ClusterRole granting `get`/`list`/`watch` on `nodes` and `namespaces` and `get`/`list` on `metrics.k8s.io` nodes/pods (the pre-bundle demo grants, verbatim); `full` binds the built-in `cluster-admin` ClusterRole. `mode: full` SHALL never be a default anywhere (including demo mode) and SHALL be documented as granting the agent unrestricted cluster control. `rbac.enabled: false` SHALL render no bindings, leaving the SA powerless. Every binding SHALL name the runtime ServiceAccount in the **conversations namespace**, since that is where runtime pods and their ServiceAccount live; in single-namespace mode that resolves to the release namespace, unchanged.

#### Scenario: Readonly is the default everywhere
- **WHEN** the bundle is enabled (directly or via demo) without setting `rbac.mode`
- **THEN** only the `view` binding and the read-only ClusterRole render; no write verb is granted anywhere

#### Scenario: Full mode is an explicit opt-in
- **WHEN** `k8s-bundle.rbac.mode=full` is set
- **THEN** a ClusterRoleBinding to `cluster-admin` for the runtime SA renders in place of the read-only objects

#### Scenario: RBAC off means a powerless agent
- **WHEN** `rbac.enabled=false`
- **THEN** no bindings render and the k8s-engineer agent cannot read the cluster API

#### Scenario: Binding subject follows the runtime namespace
- **WHEN** the bundle renders with a conversations namespace distinct from the release namespace
- **THEN** every ClusterRoleBinding subject names the runtime ServiceAccount in the conversations namespace

## ADDED Requirements

### Requirement: Bundle identity CRs stay in the control namespace
The bundle's `AgentProfile`, `AgentRuntime`, `Pipeline`, `MCPToolset`,
`MCPConfig`, `SignalSource` and `SignalAdapter` objects SHALL render in the
release (control) namespace, and only the runtime ServiceAccount and the
subjects of runtime RBAC SHALL follow the split. The bundle's MCP server
workload SHALL stay in the control namespace, and its URL SHALL remain
reachable from runtime pods — which, with default NetworkPolicies enabled,
requires the bundle to contribute its own egress allowance.

#### Scenario: Wiring renders in the control namespace
- **WHEN** the bundle is enabled in split mode
- **THEN** its profile, runtime, pipeline, toolset, MCP config, signal source and signal adapter objects are all in the release namespace

#### Scenario: Bundle MCP server stays reachable
- **WHEN** the bundle is enabled with NetworkPolicies on
- **THEN** an egress allowance for the bundle's MCP server Service renders, and an agent using the k8s toolset can reach it
