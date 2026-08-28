## MODIFIED Requirements

### Requirement: The agent definition's declared tools are honoured
When a work unit names an agent and the profile's checkout contains that agent's definition, the runtime SHALL read its YAML frontmatter and treat a declared `tools:` list as that agent's own capability declaration. WHERE the definition lives is the RUNTIME's fact, not the contract's: `runtime-claude` and `runtime-ollama` read `.claude/agents/<agent>.md`, `runtime-copilot` reads `.github/agents/<agent>.agent.md`, and another backend may read somewhere else. A definition that is absent, has no frontmatter, or declares no `tools:` SHALL contribute nothing — not an error and not a default. Frontmatter the runtime cannot parse SHALL be logged and treated as contributing nothing, never as a failure that blocks the run: an unreadable role file must not stop an agent from answering.

"Contributes nothing" is the CONTRACT's rule and binds every runtime, including one whose vendor reads an absent declaration as a grant of everything. A runtime SHALL neutralise such a default at the boundary — passing the composed allowlist explicitly, empty when the composition produced nothing — rather than letting the vendor's reading of an omitted field widen a route nobody widened.

#### Scenario: A declared tool list participates
- **WHEN** an agent definition declares `tools:` and its conversation's wiring grants a toolset
- **THEN** the agent's declared tools take part in the allowlist the runtime passes, rather than being read as narrative and ignored

#### Scenario: No definition, no contribution
- **WHEN** the profile has no repository, or the named agent's file does not exist at the path this runtime reads
- **THEN** the agent contributes no tools and the wiring's toolsets stand alone

#### Scenario: Unparseable frontmatter degrades, never blocks
- **WHEN** an agent definition's frontmatter cannot be parsed
- **THEN** the runtime logs it, treats the agent as declaring no tools, and runs

#### Scenario: Each runtime reads its own vendor's path
- **WHEN** one profile's repository is executed by a route on a claude runtime and by a route on a copilot runtime
- **THEN** each runtime reads the definition path its vendor defines, and neither treats the other's absent path as an error

#### Scenario: A permissive vendor default does not become a grant
- **WHEN** a runtime's vendor would read an agent definition with no `tools:` as granting every tool
- **THEN** that runtime passes the composed allowlist explicitly instead, so the agent still contributes nothing and an empty composition stays empty
