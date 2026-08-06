# ha-bundle

## ADDED Requirements

### Requirement: HA bundle ships as a condition-gated subchart, disabled by default
The chart SHALL include an `ha-bundle` subchart (parent dependency with `condition: ha-bundle.enabled`, default **false**) rendering, when enabled: an observability `MCPConfig`, an `AgentProfile` (repository, HA MCP server, tool allowlist, agent env), and a `SignalSource` for HA log alerts referencing the bundle's profile. When disabled the subchart SHALL render nothing.

#### Scenario: Disabled by default
- **WHEN** the parent chart renders with default values
- **THEN** no ha-bundle objects appear in the output

#### Scenario: Enabled renders the full stack
- **WHEN** rendered with `ha-bundle.enabled=true` and populated URLs/secret names
- **THEN** the output contains the MCPConfig, the AgentProfile (with MCP configRef + inline HA server), and the SignalSource wired to the profile

### Requirement: Everything configurable, partial setups valid
Every object name, URL, secret reference, allowlist, grouping field, and the SignalSource `type`/`config` SHALL come from values (defaults mirroring `config/samples/`). Empty-valued optional pieces (a missing observability URL, no repository, no HA server, no channelRef) SHALL be omitted from the rendered CRs, which SHALL remain valid.

#### Scenario: Repo-less partial setup
- **WHEN** rendered with only the VictoriaLogs URL set and no repository or HA server values
- **THEN** the AgentProfile renders without repository/HA-server blocks and the MCPConfig contains only the victorialogs server — all objects valid

#### Scenario: Signal type switchable by values
- **WHEN** `ha-bundle.signalSource.type` is set to a future adapter type with its opaque config
- **THEN** the SignalSource renders that type/config verbatim with the bundle's profile and grouping unchanged

### Requirement: Secrets referenced by name, never created
The subchart SHALL render secret references (`secretKeyRef` names/keys) from values and SHALL NOT create or template any Secret; prerequisites (HA tokens, repo SSH key — LF-only) SHALL be documented with the values.

#### Scenario: No secret manifests
- **WHEN** the subchart renders enabled with all features on
- **THEN** the output contains zero `kind: Secret` objects, and all credential fields are `valueFrom` references to the configured names
