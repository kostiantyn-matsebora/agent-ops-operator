# k8s-mcp-tooling

## ADDED Requirements

### Requirement: The bundle ships Kubernetes MCP access as a referencable config and toolset
When its MCP component is active, `k8s-bundle` SHALL render an `MCPConfig` CR (default name `k8s-api`) whose single server entry uses the FIXED server key `kubernetes` — the key is the `mcp__kubernetes__*` tool namespace named by toolsets and allowlists, and SHALL NOT be values-configurable — with a values-configured URL and values-passthrough `headers` supporting `valueFrom` secret references (the manager reads no Secrets). It SHALL also render an `MCPToolset` (default name `k8s-observability`) granting that namespace. An active component whose URL is required but empty SHALL fail rendering with a message naming the value, rather than producing a config that points nowhere.

#### Scenario: Config and toolset are referencable from any wiring
- **WHEN** the bundle renders with the MCP component active and a URL supplied
- **THEN** both CRs exist and can be named from any Pipeline's `mcpConfigs` and `toolsets` bindings, including Pipelines the bundle did not create

#### Scenario: The server key is not values-configurable
- **WHEN** an operator attempts to rename the server key through values
- **THEN** no values path exists to do so — the rendered key stays `kubernetes`, because changing it would silently strip `mcp__kubernetes__*` from every allowlist naming it

#### Scenario: Missing endpoint fails loudly
- **WHEN** the MCP component is active, no server workload is deployed, and the URL is empty
- **THEN** rendering fails naming the required value

### Requirement: The optional server workload runs under its own reviewable identity
The bundle MAY deploy the Kubernetes MCP server itself, off by default. When deployed it SHALL run under a ServiceAccount distinct from the agent runtime's, with its own values-gated RBAC defaulting to read-only, and the bundle SHALL render the Service the `MCPConfig` URL defaults onto. The agent's reach through MCP is therefore the intersection of that ServiceAccount's permissions and the tools its allowlist grants — two independent walls, each reviewable alone, unlike the kubectl path where the runtime ServiceAccount's RBAC is the only one.

#### Scenario: Server identity is separate from the agent identity
- **WHEN** the server component is enabled with default RBAC
- **THEN** the server pod runs under its own ServiceAccount, and revoking that ServiceAccount's permissions removes the agent's MCP reach without touching the runtime ServiceAccount

#### Scenario: Deployed server supplies the default endpoint
- **WHEN** the server component is enabled and the MCP config URL is left empty
- **THEN** the rendered `MCPConfig` points at the deployed Service instead of failing the render

#### Scenario: Off by default
- **WHEN** the bundle renders with defaults
- **THEN** no MCP server workload, Service, ServiceAccount, or RBAC is created — only the config and toolset CRs

### Requirement: The bundle wires its own tooling
When the MCP component is active and the bundle renders its events Pipeline, that Pipeline SHALL bind the bundle's `MCPConfig` and `MCPToolset` in `merge` mode, so a default install needs no manual wiring stanza to give event-driven conversations Kubernetes MCP tools. `merge` SHALL be used so the profile's own tools survive alongside the MCP ones.

#### Scenario: No manual step for the bundle's own lane
- **WHEN** the bundle renders with the MCP component and the events source active
- **THEN** the rendered Pipeline carries both tooling stanzas, and conversations it originates can call `mcp__kubernetes__*` tools in addition to the profile's own

#### Scenario: Other entry points are unaffected
- **WHEN** a conversation reaches the same profile through `POST /task` with no pipeline named
- **THEN** it carries no bindings and uses the profile's own tools — wiring-level tooling applies to the wiring, as specified

### Requirement: kubectl remains the fallback path
This change SHALL NOT remove `kubectl` from the runtime image, change the shipped profile's `allowedTools`, or make MCP a prerequisite for the bundle to function. An install with the MCP component disabled SHALL behave exactly as it did before this change.

#### Scenario: MCP-less install is unchanged
- **WHEN** the bundle renders with the MCP component disabled
- **THEN** the agent reaches the cluster through `Bash` and `kubectl` under the runtime ServiceAccount, exactly as before

#### Scenario: Both paths coexist
- **WHEN** the MCP component is active
- **THEN** the agent has both MCP tools and kubectl available, and neither is disabled by the other
