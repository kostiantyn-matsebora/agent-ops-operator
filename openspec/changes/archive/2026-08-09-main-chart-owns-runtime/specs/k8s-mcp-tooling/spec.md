# k8s-mcp-tooling — delta

## MODIFIED Requirements

### Requirement: The optional server workload runs under its own reviewable identity
The bundle SHALL deploy the Kubernetes MCP server itself, **on by default** alongside the MCP config component — the two flip together so the config's URL always has a Service to default onto. When deployed it SHALL run under a ServiceAccount distinct from the agent runtime's — the render SHALL FAIL if they are set equal — with its own values-gated RBAC, and the bundle SHALL render the Service the `MCPConfig` URL defaults onto. Because the runtime ServiceAccount is now the release-wide one at `global.agentops.runtime.serviceAccountName`, that equality guard SHALL compare against the global value.

The agent's reach through MCP is therefore the intersection of that ServiceAccount's permissions and the tools its allowlist grants: two independent walls, each reviewable alone, unlike the kubectl path where the runtime ServiceAccount's RBAC is the only one.

The server's read-only mode and its ServiceAccount's RBAC SHALL default to **deriving from the release's single runtime RBAC mode** (`global.agentops.runtime.rbacMode`) rather than being set independently: `full` SHALL yield a write-capable server under a `full` ServiceAccount, and every other mode — including `none` and unset — SHALL yield a read-only server under a `readonly` ServiceAccount. An explicitly set value SHALL win over the derivation.

The derivation exists because the two settings are bound by an invariant operators previously maintained by hand: a read-only MCP server under a `full` agent pushes every write back onto kubectl, which is the single-wall path this component exists to replace. Read-only mode SHALL still be understood as the FIRST wall — an unregistered tool is uncallable, not merely unlisted — and the toolset split SHALL remain the second, so mutations require a Pipeline to bind the mutating toolset deliberately no matter what the server serves.

Derivation SHALL NOT be presented as equivalent to independence: the documentation SHALL state that widening the agent's RBAC to `full` widens the server too unless overridden, and SHALL name the override that recovers a write-capable agent with a read-only MCP path.

#### Scenario: Server identity is separate from the agent identity
- **WHEN** the server component is enabled
- **THEN** the server pod runs under its own ServiceAccount, and revoking that ServiceAccount's permissions removes the agent's MCP reach without touching the runtime ServiceAccount

#### Scenario: Sharing the runtime identity is refused
- **WHEN** the server's ServiceAccount is set equal to `global.agentops.runtime.serviceAccountName`
- **THEN** the render fails, because collapsing the two identities removes the only thing this component adds over kubectl

#### Scenario: Deployed server supplies the default endpoint
- **WHEN** the server component is enabled and the MCP config URL is left empty
- **THEN** the rendered `MCPConfig` points at the deployed Service instead of failing the render

#### Scenario: One knob configures both identities coherently
- **WHEN** `global.agentops.runtime.rbacMode=full` is set and no MCP value is set
- **THEN** the server renders in write mode under a `full` ServiceAccount and the mutating toolset renders, so no install has to restate `full` a second and third time

#### Scenario: Read-only stays the default posture
- **WHEN** the bundle renders with defaults, including demo mode
- **THEN** the MCP CRs and the server workload render, the server runs `--read-only` under a readonly ServiceAccount, no mutating toolset exists, and the render succeeds

#### Scenario: The separation is recoverable
- **WHEN** `rbacMode=full` and `mcpServers.readOnly=true` is set explicitly
- **THEN** the agent keeps write power through its own identity while the MCP path serves reads only, and no mutating toolset renders

#### Scenario: The endpoint guard still bites
- **WHEN** `mcp.enabled=true` with `mcpServers.enabled=false` and no `mcp.url`
- **THEN** the render fails naming the missing endpoint, because an `MCPConfig` pointing nowhere silently costs agents their tools
