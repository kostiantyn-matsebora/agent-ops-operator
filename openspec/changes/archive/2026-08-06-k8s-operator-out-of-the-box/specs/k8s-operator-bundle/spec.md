# k8s-operator-bundle

## ADDED Requirements

### Requirement: Chart ships a ready k8s-operator agent, enabled by default
With `k8sOperator.enabled: true` (the default) the chart SHALL render: an `AgentRuntime k8s-operator` (the derived image, a dedicated ServiceAccount, credential env from `k8sOperator.credentialsSecret` via `valueFrom`, global idle TTL, home volume when persistence is enabled) and an `AgentProfile k8s-operator` (repo-less, `runtimeRef: k8s-operator`, inline stdio Kubernetes MCP from the baked-in server, allowlist covering shell tooling + `mcp__kubernetes__*`). With `k8sOperator.enabled: false` the chart SHALL render none of the bundle. The agent SHALL be addressable immediately (`POST /task` profile `k8s-operator`; `/k8s-operator <task>` on channels).

#### Scenario: Out of the box
- **WHEN** the chart is installed with default values and the claude credential Secret exists
- **THEN** `POST /task {"profile":"k8s-operator","task":"list nodes"}` produces a conversation answered via the Kubernetes MCP/kubectl under read-only rights

#### Scenario: Fully removable
- **WHEN** the chart renders with `k8sOperator.enabled: false`
- **THEN** no k8s-operator runtime, profile, ServiceAccount, or RBAC objects are produced

### Requirement: Read-only by default, full rights by explicit flag
The bundle SHALL run under its own dedicated ServiceAccount, never the shared runtime SA. With `k8sOperator.fullAccess: false` (the default, demo mode) the SA SHALL be bound to read-only rights (`view` plus nodes/namespaces/metrics reads) and the profile's MCP server SHALL run in non-destructive mode. With `k8sOperator.fullAccess: true` the SA SHALL be bound to `cluster-admin` and the MCP restriction SHALL be lifted. The flag flips bindings and MCP mode only — no other object changes.

#### Scenario: Demo mode cannot mutate
- **WHEN** `fullAccess` is false and the agent attempts a destructive operation via kubectl
- **THEN** the API server denies it (read-only RBAC), and the MCP toolset never offered a destructive tool

#### Scenario: Full rights on one flag
- **WHEN** the release is upgraded with `k8sOperator.fullAccess=true`
- **THEN** the dedicated SA gains the cluster-admin binding, read-only bindings are gone, and new runtime pods get the unrestricted MCP toolset

#### Scenario: Trust isolation
- **WHEN** the bundle is enabled alongside the demo advisor and user-defined profiles
- **THEN** the k8s-operator SA's rights are disjoint from the demo SA and the shared runtime SA (no shared bindings)
