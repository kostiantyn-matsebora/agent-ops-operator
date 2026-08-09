# k8s-mcp-tooling Specification

## Purpose
Kubernetes access as MCP tools in `k8s-bundle`: the `MCPConfig` and the risk-split `MCPToolset` CRs any Pipeline can bind, and the optional in-cluster server workload whose own ServiceAccount is the second of the two walls between an agent and the API.
## Requirements
### Requirement: The bundle ships Kubernetes MCP access as a referencable config and toolsets
When its MCP component is active, `k8s-bundle` SHALL render an `MCPConfig` CR (default name `k8s-api`) whose single server entry uses the FIXED server key `kubernetes` — the key is the `mcp__kubernetes__*` tool namespace named by toolsets and allowlists, and SHALL NOT be values-configurable — with a values-configured URL, a values-selected transport (streamable HTTP or SSE), and values-passthrough `headers` supporting `valueFrom` secret references (the manager reads no Secrets). An active component whose URL is required but empty SHALL fail rendering with a message naming the value, rather than producing a config that points nowhere.

#### Scenario: Config and toolsets are referencable from any wiring
- **WHEN** the bundle renders with the MCP component active and a URL supplied
- **THEN** the CRs exist and can be named from any Pipeline's `mcpConfigs` and `toolsets` bindings

#### Scenario: The server key is not values-configurable
- **WHEN** an operator attempts to rename the server key through values
- **THEN** no values path exists to do so — the rendered key stays `kubernetes`, because changing it would silently strip `mcp__kubernetes__*` from every allowlist naming it

#### Scenario: Missing endpoint fails loudly
- **WHEN** the MCP component is active, no server workload is deployed, and the URL is empty
- **THEN** rendering fails naming the required value

### Requirement: Reads and mutations are separate toolsets
The bundle SHALL render its tool grants as TWO `MCPToolset` CRs split by risk, mirroring the built-in `observe`/`shell`/`edit` split: a read set (default `k8s-observability`) and a mutating set (default `k8s-admin`). Tool patterns SHALL be ENUMERATED rather than a `mcp__kubernetes__*` wildcard, because a wildcard spans both halves and would defeat the split it is meant to express.

The read set SHALL render whenever the component is active. The mutating set SHALL render only when a server that actually REGISTERS mutating tools exists — following the deployed server's mode by default, with an explicit values override for operators pointing at a server this chart did not deploy. Granting tool names that resolve to nothing is how an allowlist rots into fiction.

#### Scenario: Read-only server grants no mutating tools
- **WHEN** the bundle deploys the MCP server in read-only mode
- **THEN** only the read toolset renders, and no toolset names a mutating tool

#### Scenario: Write-mode server renders both
- **WHEN** the deployed server runs without read-only
- **THEN** both toolsets render, and binding writes is a separate decision from binding reads

#### Scenario: A route can read without mutating
- **WHEN** a Pipeline binds the read toolset alone
- **THEN** its conversations can inspect the cluster and cannot change it, even though the server serves mutating tools to other routes

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

### Requirement: kubectl remains the fallback path
This change SHALL NOT remove `kubectl` from the runtime image or make MCP a prerequisite for the bundle to function. An install with the MCP component disabled SHALL behave exactly as it did before this change.

MCP SHALL NOT be assumed to cover everything kubectl does: the served tool set has no patch semantics, no rollout, drain, wait, port-forward or text processing, so a route that grants MCP writes MAY still need shell access for what those tools cannot express.

#### Scenario: MCP-less install is unchanged
- **WHEN** the bundle renders with the MCP component disabled
- **THEN** the agent reaches the cluster through `Bash` and `kubectl` under the runtime ServiceAccount, exactly as before

#### Scenario: Both paths coexist
- **WHEN** the MCP component is active
- **THEN** the agent has both MCP tools and kubectl available, and neither is disabled by the other
