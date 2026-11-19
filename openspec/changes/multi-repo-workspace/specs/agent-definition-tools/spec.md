## MODIFIED Requirements

### Requirement: The agent definition's declared tools are honoured
When a work unit names an agent, or carries the resolved `agentFile` path, and the workspace contains that definition, the runtime SHALL read its YAML frontmatter and treat a declared `tools:` list as that agent's own capability declaration. WHERE the definition lives for the name form is the RUNTIME's fact, not the contract's. For the `agentFile` form the manager states the location, as an absolute path under `/workspace`, and a runtime SHALL read exactly that file and SHALL refuse a path outside `/workspace` or reaching it through a symlink out of its repository. For the name form: `runtime-claude` and `runtime-ollama` read `.claude/agents/<agent>.md`, `runtime-copilot` reads `.github/agents/<agent>.agent.md`, and another backend may read somewhere else. A definition that is absent, has no frontmatter, or declares no `tools:` SHALL contribute nothing — not an error and not a default. Frontmatter the runtime cannot parse SHALL be logged and treated as contributing nothing, never as a failure that blocks the run: an unreadable role file must not stop an agent from answering.

"Contributes nothing" is the CONTRACT's rule and binds every runtime, including one whose vendor reads an absent declaration as a grant of everything. A runtime SHALL neutralise such a default at the boundary — passing the composed allowlist explicitly, empty when the composition produced nothing — rather than letting the vendor's reading of an omitted field widen a route nobody widened.

#### Scenario: A declared tool list participates
- **WHEN** an agent definition declares `tools:` and its conversation's wiring grants a toolset
- **THEN** the agent's declared tools take part in the allowlist the runtime passes, rather than being read as narrative and ignored

#### Scenario: No definition, no contribution
- **WHEN** the profile has no repository, or the named agent's file does not exist at the path this runtime reads, or no `agentFile` is declared
- **THEN** the agent contributes no tools and the wiring's toolsets stand alone

#### Scenario: Unparseable frontmatter degrades, never blocks
- **WHEN** an agent definition's frontmatter cannot be parsed
- **THEN** the runtime logs it, treats the agent as declaring no tools, and runs

#### Scenario: Each runtime reads its own vendor's path
- **WHEN** one profile's repository, named by agent name, is executed by a route on a claude runtime and by a route on a copilot runtime
- **THEN** each runtime reads the definition path its vendor defines, and neither treats the other's absent path as an error

#### Scenario: A permissive vendor default does not become a grant
- **WHEN** a runtime's vendor would read an agent definition with no `tools:` as granting every tool
- **THEN** that runtime passes the composed allowlist explicitly instead, so the agent still contributes nothing and an empty composition stays empty

#### Scenario: The agent file is read from where it is declared

- **WHEN** a work unit carries `agentFile: /workspace/infra/agents/ops.md`
- **THEN** the runtime reads that file whatever vendor path convention it follows for a name

## ADDED Requirements

### Requirement: An agent file path is absolute, inside the workspace and chosen by the manager

When a profile or capability declares `agentFile`, the manager SHALL resolve it against the resolved repository list and place the absolute path in the work unit's `agentFile`. The work unit SHALL carry `agent` or `agentFile`, never both. A runtime SHALL refuse a value that is not absolute, contains `..`, or resolves outside the workspace, and SHALL report the refusal as a failed run naming the path. Each runtime SHALL decide how to apply the file. What a runtime does with the file's frontmatter is the requirement "The agent definition's declared tools are honoured" above, and what it does with the body belongs to the separate change `agent-definition-adopted-by-runtime`. This requirement states only where the file is.

#### Scenario: A traversal is refused

- **WHEN** a work unit carries `agentFile: /workspace/../etc/passwd`
- **THEN** the run fails naming the path and nothing outside the workspace is read

#### Scenario: Name and file are exclusive

- **WHEN** a work unit is built for a profile declaring `agentFile`
- **THEN** it carries `agentFile` and no `agent`

