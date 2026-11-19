## MODIFIED Requirements

### Requirement: AgentProfile declares identity, never capability

`AgentProfile` SHALL carry no capability fields: `spec.allowedTools` and
`spec.mcp` do not exist. What remains is IDENTITY — repositories, agent role,
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

### Requirement: A repository-less profile can carry an inline role
An `AgentProfile` MAY declare inline role text. When present the runtime SHALL
APPEND it to the agent's system prompt rather than replacing it, so the
runtime's own instructions survive.

This exists for profiles with NO repository: an agent definition file is
resolved from a checkout (`agent` or `agentFile`), so a profile without one can name no definition and
would otherwise run with no role at all. A profile WITH a repository SHOULD
carry its role in the definition file instead, which is version-controlled and
can declare its own `tools:`.

Inline role text is IDENTITY, never capability. It SHALL NOT affect the
allowlist: what an agent may call remains exclusively what the Pipeline's
toolsets grant, composed with the definition's declared tools. Role text that
names a tool grants nothing.

#### Scenario: A repo-less agent still has a role
- **WHEN** a profile with no repository declares inline role text and a
  conversation runs
- **THEN** the agent behaves according to that role, and the runtime's own
  system prompt is still in effect

#### Scenario: Role text grants nothing
- **WHEN** inline role text instructs the agent to use a tool the Pipeline did
  not grant
- **THEN** the tool is absent from the allowlist and the call is denied, exactly
  as if the text had not mentioned it

#### Scenario: Absent by default
- **WHEN** a profile declares no inline role
- **THEN** nothing is appended and behavior is unchanged

## ADDED Requirements

### Requirement: A profile declares its repositories and where its agent definition lives

`AgentProfile.spec` SHALL accept `repositories[]`, `agentFile` and `workingDir` with the semantics `multi-repo-workspace` states. `spec.repository` SHALL remain as a DEPRECATED one-entry shorthand and SHALL NOT be set with `repositories`. `spec.agent` and `spec.agentFile` SHALL be mutually exclusive. These fields are IDENTITY: they say where the agent's definition and material live, never what the agent may call or run as.

#### Scenario: A profile with two repositories and a file in the second

- **WHEN** a profile lists `app` and `infra` and sets `agentFile: {repo: infra, path: agents/ops.md}`
- **THEN** the agent definition is read from `infra`, and the profile grants no tool or identity by saying so

#### Scenario: The identity fields carry no capability

- **WHEN** a profile declares `repositories`
- **THEN** the allowlist, the MCP servers and the service account still come only from the originating Pipeline

