# profile-is-identity — delta

## REMOVED Requirements

### Requirement: A capability-only Pipeline declares a profile's baseline
**Reason**: The baseline existed only to serve conversations addressed to a profile rather than to a route. `POST /task` now addresses a Pipeline, and `chat-signal-origination` routes chat through a claiming Pipeline, so no path lacks one. A per-profile default would override what the operator wrote on the route.

**Migration**: Delete capability-only Pipelines (a `profileRef` with no sources and no channels), or give them sources/channels so they route something. Any conversation that relied on a baseline must now originate from a Pipeline that declares the capabilities itself.

## MODIFIED Requirements

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
