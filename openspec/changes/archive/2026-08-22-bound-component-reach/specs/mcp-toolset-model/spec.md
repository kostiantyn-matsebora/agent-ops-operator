## ADDED Requirements

### Requirement: The effective allowlist is enforceable without the agent's cooperation
The tool access a conversation's wiring grants SHALL be enforceable at a point the agent does not control. Where the runtime composes and passes `--allowedTools` to a CLI running beside the agent, that composition SHALL be understood as CONFIGURATION of a cooperating agent, not as a boundary: an agent able to execute commands can reach a bound MCP server directly and call anything that server registers. An installation SHALL therefore be able to make the same access decision binding on a non-cooperating agent, and the two SHALL derive from the SAME wiring — the bound toolsets and their mode — so that what is configured and what is enforced can never disagree.

Documentation of capability resolution SHALL NOT describe the CLI allowlist as the boundary on an agent's reach without naming this distinction.

#### Scenario: A shell-capable agent is still bound by its toolset
- **WHEN** a conversation whose bound toolsets grant only read tools uses a shell to call a mutating tool on a bound MCP server directly
- **THEN** the call is refused, because the wiring's access decision is enforced outside the agent

#### Scenario: Configured and enforced access come from one source
- **WHEN** a pipeline's bound toolsets change and a new work unit is dispatched
- **THEN** both the allowlist passed to the runtime and the access enforced against the agent reflect the same change, with no separate configuration to keep in step

#### Scenario: Enforcement is not claimed where it does not exist
- **WHEN** an installation has not enabled enforcement outside the agent
- **THEN** the documented guarantee is the configured allowlist of a cooperating agent, and is described as such
