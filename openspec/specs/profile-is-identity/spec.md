# profile-is-identity

## Purpose

`AgentProfile` answers WHO the agent is — repository, role, prompts, its own credentials, limits, resources, runtime preference — and never WHAT it may do. Capability comes exclusively from the `Pipeline` routing a conversation — the Pipeline that originates it declares what its conversations may do, and there is no per-profile default.

## Requirements

### Requirement: AgentProfile declares identity, never capability
`AgentProfile` SHALL carry no capability fields: `spec.allowedTools` and `spec.mcp` do not exist. What remains is identity and execution preference — repository, agent role, prompts, env (the agent's own credentials, by `valueFrom`), limits, resources, and `runtimeRef`. What an agent MAY DO SHALL come exclusively from the `Pipeline` that originated its conversation, with no per-profile default and no inheritance: a Pipeline's declared bindings ARE its conversations' capabilities, and a Pipeline that declares none grants none.

#### Scenario: Profiles carry no tool or MCP fields
- **WHEN** an `AgentProfile` is applied with `allowedTools` or `mcp` set
- **THEN** those fields are not part of the schema and are pruned

#### Scenario: One profile, differently-capable routes
- **WHEN** two Pipelines route to one profile with different `toolsets` and `mcpConfigs`
- **THEN** conversations from each get exactly that Pipeline's capabilities, and the profile is identical for both

#### Scenario: A Pipeline that declares nothing grants nothing
- **WHEN** a conversation originates from a Pipeline with no `toolsets` and no `mcpConfigs`
- **THEN** it dispatches with an empty allowlist and no MCP servers — there is no profile default to fall back to

#### Scenario: The agent's own credentials stay with its identity
- **WHEN** a profile declares `env` entries with `valueFrom`
- **THEN** they are unaffected — they are the agent's credentials, not the route's capabilities
