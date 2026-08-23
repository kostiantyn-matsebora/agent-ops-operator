## MODIFIED Requirements

### Requirement: AgentProfile declares identity, never capability

`AgentProfile` SHALL carry no capability fields: `spec.allowedTools` and
`spec.mcp` do not exist. What remains is IDENTITY — repository, agent role,
prompts, env (the agent's own credentials, by `valueFrom`), limits and
resources.

IT SHALL NOT SELECT WHAT EXECUTES IT. `spec.runtimeRef` is DEPRECATED and moves
to the `Pipeline`, because an `AgentRuntime` carries the ServiceAccount an agent
runs as — so a profile choosing one chose the agent's power in the cluster.

That placement made profile-edit rights into service-account-choice rights,
while a profile is prompts and a repo ref and a Pipeline already grants tools.
Whoever is trusted to grant capabilities is more qualified to choose an
execution identity, not less.

What an agent MAY DO, and WHAT IT RUNS AS, SHALL come exclusively from the
`Pipeline` that originated its conversation, with no per-profile default and no
inheritance.

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

#### Scenario: Identity states no execution preference

- **WHEN** a profile is read to answer what an agent may do or run as
- **THEN** it answers neither — both come from the originating Pipeline

#### Scenario: Two routes, one profile, different power

- **WHEN** one profile is routed by two Pipelines naming different service
  accounts
- **THEN** its conversations run with different cluster power, and the profile
  is unchanged and uncloned
