# k8s-mcp-tooling — delta

## REMOVED Requirements

### Requirement: kubectl remains the fallback path
**Reason**: This change removes `kubectl` from the runtime image, which is exactly what that requirement promised would not happen. It was written as the deliberate scope limit of `k8s-mcp-tooling` — MCP added a path without taking one away — and it held until the evidence gate in this change confirmed the MCP path stands alone. Keeping it would leave two requirements in direct contradiction.

**Migration**: Replaced by "MCP is the only cluster path" below. Operators who need a CLI keep one by deriving a runtime image and pointing `AgentRuntime.spec.image` at it; the recipe is in `docs/concepts.md`. Pinning the previous runtime image tag holds the old behaviour unchanged.

## ADDED Requirements

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
