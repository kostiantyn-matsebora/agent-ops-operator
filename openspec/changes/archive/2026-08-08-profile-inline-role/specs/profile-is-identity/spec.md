# profile-is-identity — delta

## ADDED Requirements

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
