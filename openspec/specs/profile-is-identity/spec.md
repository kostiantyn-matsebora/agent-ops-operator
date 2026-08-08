# profile-is-identity

## Purpose

`AgentProfile` answers WHO the agent is — repository, role, prompts, its own credentials, limits, resources, runtime preference — and never WHAT it may do. Capability comes exclusively from the `Pipeline` routing a conversation, including the capability-only Pipeline that declares a profile's baseline for conversations created with no routing pipeline.

## Requirements

### Requirement: AgentProfile declares identity, never capability
`AgentProfile` SHALL carry no capability fields: `spec.allowedTools` and `spec.mcp` are removed. What remains is identity and execution preference — repository, agent role, prompts, env (the agent's own credentials, by `valueFrom`), limits, resources, and `runtimeRef`. What an agent MAY do SHALL come exclusively from the `Pipeline` routing its conversation, so one profile can serve routes with genuinely different capabilities without being cloned or edited.

#### Scenario: Profiles carry no tool or MCP fields
- **WHEN** an `AgentProfile` is applied with `allowedTools` or `mcp` set
- **THEN** those fields are not part of the schema and are pruned — capability declarations live on Pipelines

#### Scenario: One profile, differently-capable routes
- **WHEN** two Pipelines route to one profile with different `toolsets` and `mcpConfigs`
- **THEN** conversations from each get exactly that Pipeline's capabilities, and the profile is identical for both

#### Scenario: The agent's own credentials stay with its identity
- **WHEN** a profile declares `env` entries with `valueFrom`
- **THEN** they are unaffected — they are the agent's credentials, not the route's capabilities, and moving them would put secret references into the wiring object

### Requirement: A capability-only Pipeline declares a profile's baseline
A `Pipeline` naming a `profileRef` with no `signalSourceRefs` and no `channelRefs` SHALL declare that profile's baseline capabilities. Conversations created with no routing pipeline — `POST /task` without a `pipeline`, and `/<profile>` chat commands — SHALL resolve their capabilities from it. A routing Pipeline's own bindings SHALL take precedence for the conversations it originates. At most ONE capability-only Pipeline per profile SHALL apply; a second SHALL surface a condition on both and neither SHALL apply, rather than a silent precedence rule.

#### Scenario: The task API reaches a capable agent without naming a pipeline
- **WHEN** `POST /task` names a profile that has a capability-only Pipeline, and no `pipeline` field
- **THEN** the conversation's work unit carries that Pipeline's toolsets and MCP configs

#### Scenario: A routing pipeline overrides the baseline
- **WHEN** a signal routes through a Pipeline whose bindings differ from the profile's capability-only Pipeline
- **THEN** the conversation gets the routing Pipeline's capabilities, not the baseline

#### Scenario: Chat commands resolve the baseline, not the channel's route
- **WHEN** a user runs `/<profile> <task>` on a channel wired to a Pipeline for a DIFFERENT profile
- **THEN** the conversation gets the named profile's baseline capabilities — the channel Pipeline's capabilities belong to its own profile and would be the wrong agent's

#### Scenario: A profile with no capability Pipeline is genuinely unwired
- **WHEN** `POST /task` names a profile with no capability-only Pipeline and no `pipeline` field
- **THEN** the conversation has no tools and no MCP, consistent with unclaimed sources dropping signals and unwired channels answering with guidance only

#### Scenario: Duplicate baselines refuse rather than guess
- **WHEN** two capability-only Pipelines name the same profile
- **THEN** both report the conflict and neither supplies capabilities, so the ambiguity is visible instead of silently resolved
