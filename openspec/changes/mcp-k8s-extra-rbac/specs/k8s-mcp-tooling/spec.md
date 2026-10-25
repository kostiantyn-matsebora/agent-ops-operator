## ADDED Requirements

### Requirement: The server's read-only RBAC accepts an additive extension list

`k8s-bundle` SHALL render an additive list of extra read rules
(`mcpServers.rbac.extraReadRules`, default empty) onto the `agentops-mcp-k8s`
ClusterRole's read rules, in BOTH `mcpServers.rbac.mode` settings
(`readonly` and `full`), so the extension does not depend on which mode an
install runs.

Each entry SHALL carry `apiGroups`, `resources`, and `verbs`, in the same
shape as the chart's existing fixed RBAC rules. The extension SHALL NOT
apply to any other ServiceAccount this chart renders — it is scoped to this
one bundle's MCP server identity.

The render SHALL FAIL, naming the offending entry, when an entry:
- names a verb outside `get`/`list`/`watch`
- names `resources: ["secrets"]` under `apiGroups: [""]`
- names `apiGroups` or `resources` equal to `"*"`
- names `apiGroups: ["rbac.authorization.k8s.io"]`
- names `apiGroups: ["agentops.dev"]`

#### Scenario: Default renders no extension

- **WHEN** `k8s-bundle` renders with `mcpServers.rbac.extraReadRules` unset
- **THEN** the `agentops-mcp-k8s` ClusterRole is identical to today's, with
  no extra rule appended

#### Scenario: An extra read rule is granted

- **WHEN** an install sets one entry for
  `apiGroups: ["operator.victoriametrics.com"]`,
  `resources: ["vmrules"]`, `verbs: ["get", "list", "watch"]`
- **THEN** the rendered `agentops-mcp-k8s` ClusterRole carries that rule
  alongside the fixed read rules

#### Scenario: The extension moves with both RBAC modes

- **WHEN** the same extra rule is set and `mcpServers.rbac.mode` is
  `readonly` in one render and `full` in another
- **THEN** both renders carry the extra rule, so the extension does not
  depend on which mode the install runs

#### Scenario: A mutating verb is refused

- **WHEN** an entry's `verbs` includes `create`, `patch`, `delete`, or any
  verb outside `get`/`list`/`watch`
- **THEN** the render fails, naming the entry, rather than silently
  granting mutation through a second path

#### Scenario: Secrets stay ungrantable through this path

- **WHEN** an entry names `apiGroups: [""]` and `resources` containing
  `secrets`
- **THEN** the render fails, naming the entry

#### Scenario: A wildcard rule is refused

- **WHEN** an entry's `apiGroups` or `resources` contains `"*"`
- **THEN** the render fails, naming the entry

#### Scenario: RBAC and CRD escalation stay ungrantable

- **WHEN** an entry names `apiGroups: ["rbac.authorization.k8s.io"]`
- **THEN** the render fails, naming the entry

#### Scenario: `agentops.dev` stays ungrantable through this path

- **WHEN** an entry names `apiGroups: ["agentops.dev"]`
- **THEN** the render fails, naming the entry
