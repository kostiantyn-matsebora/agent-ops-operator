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

### Requirement: A profile DECLARES its output contract, and the field is required

`AgentProfile.spec.outputFormat` SHALL be REQUIRED, with exactly two values:

| Value | Means |
|---|---|
| `blocks` | the shared output-format specification is appended to the agent's prompt — the block grammar, the fold, the markdown subset and a default section set |
| `none` | NOTHING is appended, and the profile's own prompt owns formatting entirely |

THERE SHALL BE NO DEFAULT, because both candidate defaults are wrong: `none`
leaves output unformatted unless the author wrote a format into the prompt, and
`blocks` shapes output by something the author never asked for. The author
declares it.

Making it required SHALL break `kubectl apply` on a profile that omits it. That
is intended: the failure names the valid values, and a profile whose output
contract is unstated is the condition this field exists to end.

This is IDENTITY, never capability — it shapes how the agent SPEAKS, not what it
may call. It SHALL NOT affect the allowlist or the MCP servers, which remain
exclusively the originating Pipeline's.

THE DECLARATION SHALL GATE THE PROMPT ONLY. Whether output is parsed into blocks
is not a profile decision — an adapter parses whatever it is given, so a profile
declaring `none` whose agent emits tags anyway is still rendered as blocks.

It SHALL NOT gate the operator's unconditional prompt content. Text stating how
the system handles a reply — that the printed answer IS the deliverable, and
that the agent never posts to a transport itself — is a fact about the system
and SHALL be injected whatever this field says.

#### Scenario: A profile without the field is refused

- **WHEN** an `AgentProfile` is applied with no `outputFormat`
- **THEN** the API rejects it, naming `blocks` and `none`

#### Scenario: Declaring none leaves formatting to the profile

- **WHEN** a profile declares `none` and its own prompt describes an output
  structure
- **THEN** no shared specification is injected, and the agent follows its profile

#### Scenario: The declaration grants nothing

- **WHEN** a profile declares `blocks`
- **THEN** the conversation's tools and MCP servers are identical to the same
  profile declaring `none`

#### Scenario: Declining the spec does not decline the parse

- **WHEN** a profile declares `none` and its agent emits block tags anyway
- **THEN** the surface renders them as blocks, because parsing follows the TEXT

### Requirement: A repository-less profile can carry an inline role
An `AgentProfile` MAY declare inline role text. When present the runtime SHALL
APPEND it to the agent's system prompt rather than replacing it, so the
runtime's own instructions survive.

This exists for profiles with NO repository: an agent definition file is
resolved from the checkout, so a profile without one can name no definition and
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

