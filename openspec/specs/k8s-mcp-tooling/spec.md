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

The agent's reach through MCP is therefore the intersection of that ServiceAccount's permissions and the tools its allowlist grants: two independent walls, each reviewable alone, unlike the kubectl path where the runtime ServiceAccount's RBAC is the only one. The SECOND wall SHALL be qualified wherever it is claimed: the allowlist is applied by the CLI running beside the agent, so it binds a COOPERATING agent only. An agent able to execute commands can reach this server directly and call anything it registers, leaving the server's ServiceAccount as the sole remaining wall. Where that matters, the wall is restored by enforcing the toolset outside the agent's control, and the bundle's risk-split toolsets SHALL NOT be documented as bounding a shell-capable agent unless such enforcement is in place.

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

#### Scenario: The second wall is qualified where it is claimed
- **WHEN** the bundle's two-wall property is documented
- **THEN** it states that the toolset wall binds a cooperating agent, and names what remains when the agent has a shell

#### Scenario: A shell-capable agent meets only the server identity by default
- **WHEN** a pipeline binds only the read toolset to an agent that also holds shell access, with no enforcement outside the agent
- **THEN** the agent can reach the deployed server directly, and what it may do there is decided solely by the server's ServiceAccount

### Requirement: MCP is the only cluster path
The runtime image SHALL carry no Kubernetes CLI, so the `mcp__kubernetes__*` tools a Pipeline binds SHALL be the only way an agent reaches the Kubernetes API. `Bash` SHALL remain useful for the workspace and SHALL NOT imply cluster access.

The served tool set SHALL NOT be assumed to cover everything a CLI does: there is no patch semantics, no rollout, drain, wait or port-forward, no permission check, and no text processing over results. What those tools cannot express, an agent with only MCP cannot do — and SHALL say so rather than improvise.

An operator who needs a CLI SHALL be able to derive a runtime image and point `AgentRuntime.spec.image` at it. That path SHALL be documented, including its cost: a CLI authenticates as the runtime ServiceAccount, so its reach is that SA's RBAC handed over whole — one wall, where the MCP path has the server's own identity and the toolset allowlist.

#### Scenario: No CLI in the image
- **WHEN** a runtime pod is inspected
- **THEN** no Kubernetes CLI binary is present, and cluster access is available only through bound MCP tools

#### Scenario: Shell without cluster reach
- **WHEN** a Pipeline binds the shell toolset but no Kubernetes MCP tools
- **THEN** the agent can work in its checkout and cannot reach the cluster API

#### Scenario: A gap is reported, not worked around
- **WHEN** a task needs an operation no bound tool expresses
- **THEN** the agent states that plainly instead of attempting a substitute

#### Scenario: The escape hatch is supported and its cost is stated
- **WHEN** an operator needs a CLI in the runtime
- **THEN** the documentation gives a derived-image recipe and states that the CLI authenticates as the runtime ServiceAccount, collapsing the two walls to one

### Requirement: Disabling the MCP component leaves an agent that cannot see the cluster
Because no other path remains, rendering the bundle with the MCP component disabled SHALL produce a Kubernetes agent with no cluster access at all, whatever `global.agentops.runtime.rbacMode` grants. The install SHALL report this in its post-install notes rather than leaving it to be discovered by asking a question and receiving an apology.

The render SHALL NOT fail on that combination: pointing `mcp.url` at a separately operated MCP server is legitimate, and so is disabling the component deliberately during a migration.

#### Scenario: The blind install announces itself
- **WHEN** the bundle renders with the profile enabled and the MCP component disabled
- **THEN** the render succeeds and the post-install notes state that the agent cannot see the cluster, naming how to enable the component, how to point at an external server, and the derived-image alternative

#### Scenario: Broad grants nothing can exercise are called out
- **WHEN** that same install also sets the runtime RBAC mode to full
- **THEN** the notes additionally state that the mode grants cluster-admin to an identity nothing can exercise

#### Scenario: A working install is not nagged
- **WHEN** the bundle renders with the MCP component enabled
- **THEN** no such warning appears
