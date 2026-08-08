# k8s-bundle — delta

## MODIFIED Requirements

### Requirement: RBAC is read-only by default with an explicit full mode
When active, the `rbac` component SHALL bind roles to the profile's runtime ServiceAccount according to `rbac.mode`: `readonly` (default) binds the built-in `view` ClusterRole plus a bundle ClusterRole granting `get`/`list`/`watch` on `nodes` and `namespaces` and `get`/`list` on `metrics.k8s.io` nodes/pods; `full` binds the built-in `cluster-admin` ClusterRole. `mode: full` SHALL never be a default anywhere (including demo mode). `rbac.enabled: false` SHALL render no bindings.

Because the runtime image no longer ships a Kubernetes CLI, these grants are no longer directly exercisable from the agent's shell: cluster reach flows through the MCP server's own ServiceAccount and the `mcp__kubernetes__*` tools the allowlist grants. The documentation SHALL describe this as two independent walls — the MCP server's permissions and the tool allowlist — rather than presenting the runtime ServiceAccount's RBAC as the only boundary. `rbac.mode` continues to govern what any MCP-independent path could do, and SHALL be documented as such.

#### Scenario: Readonly is the default everywhere
- **WHEN** the bundle is enabled (directly or via demo) without setting `rbac.mode`
- **THEN** only the `view` binding and the read-only ClusterRole render; no write verb is granted anywhere

#### Scenario: Full mode is an explicit opt-in
- **WHEN** `k8s-bundle.rbac.mode=full` is set
- **THEN** a ClusterRoleBinding to `cluster-admin` for the runtime SA renders in place of the read-only objects

#### Scenario: RBAC off means a powerless agent
- **WHEN** `rbac.enabled=false`
- **THEN** no bindings render and the k8s-engineer agent cannot read the cluster API through that identity

#### Scenario: Cluster reach requires the MCP component
- **WHEN** the bundle renders with the MCP component disabled
- **THEN** the agent has no Kubernetes access regardless of `rbac.mode`, because the runtime image ships no cluster CLI — and the documentation states this rather than leaving an agent that silently cannot see the cluster it was installed to inspect
