# vm-bundle — delta

## MODIFIED Requirements

### Requirement: The bundle ships no profile — tool wiring targets the operator's own Pipeline
The bundle SHALL NOT render an AgentProfile (the alert-handling profile is operator-owned and typically already exists). `defaultSource.profileRef` is strictly required when the default source is enabled. When the bundle renders its default-source Pipeline it SHALL declare that Pipeline's capabilities — the bundle's `MCPConfig`s and `MCPToolset` when those components are active — because capabilities are declared per route and nothing supplies them otherwise. For a Pipeline the operator writes themselves, the documented step is the same stanza on that Pipeline.

#### Scenario: No profile objects from the bundle
- **WHEN** the bundle renders fully enabled
- **THEN** no `AgentProfile` object appears in the output

#### Scenario: Default source always requires an explicit profile
- **WHEN** `defaultSource.enabled=true` with an empty `profileRef`
- **THEN** rendering fails naming the value — there is no bundled profile to fall back to

#### Scenario: The rendered Pipeline declares what it grants
- **WHEN** the default source and the MCP components are both active
- **THEN** the rendered Pipeline binds the bundle's MCPConfigs and toolset, so alert conversations can query VictoriaLogs/VictoriaMetrics without the operator adding a stanza
