## ADDED Requirements

### Requirement: A profile decides whether the shared output format is injected

`AgentProfile` MAY declare that the shared output-format specification is
appended to its agent's prompt. When declared, the specification is injected.
When absent, NOTHING is injected and the profile's own prompt owns formatting.

This is IDENTITY, never capability — it shapes how the agent SPEAKS, not what it
may call. It SHALL NOT affect the allowlist or the MCP servers, which remain
exclusively the originating Pipeline's.

The declaration SHALL gate the prompt only. Whether output is parsed into blocks
is not a profile decision.

#### Scenario: Opting out leaves formatting to the profile

- **WHEN** a profile does not declare it and its own prompt describes an output
  structure
- **THEN** no shared specification is injected, and the agent follows its profile

#### Scenario: The declaration grants nothing

- **WHEN** a profile declares it
- **THEN** the conversation's tools and MCP servers are identical to the same
  profile without it
