## MODIFIED Requirements

### Requirement: Two profiles split the house by privilege
The bundle SHALL render two `AgentProfile` objects with distinct jobs:

- **`ha-user`** — the user of the house. It SHALL render when EITHER the MCP
  endpoint OR the read-scoped API credential is configured, and SHALL NOT render
  when neither is.
- **`ha-operator`** — the administrator. The admin API credential SHALL be its
  prerequisite: it SHALL render only when that credential is configured, and MCP
  configuration SHALL be optional for it.

Each profile SHALL carry behaviour and no reach — role prompt, connectivity
env, turn limit, optional runtime reference. Neither SHALL declare tools or
MCP servers, because an `AgentProfile` carries no capabilities. What each
agent may do comes from the Pipeline routing it.

Each profile SHALL carry an inline role prompt, because neither has a repository
to hold an agent definition.

The operator's role prompt SHALL state its describe-and-stop rule as a
CONFIRMATION GATE. The agent describes the action and waits, and a person's
reply in the thread that they want it done is the authorization to do exactly
what was described.

The prompt SHALL say the agent neither asks a second time nor tells the person
to run it themselves. A gate the agent may read as a permanent refusal is a
refusal wearing a gate's name.

#### Scenario: No configuration renders no user profile
- **WHEN** the bundle is enabled with neither an MCP endpoint nor a read credential
- **THEN** no `ha-user` profile is rendered

#### Scenario: MCP alone is enough for the user profile
- **WHEN** only the MCP endpoint is configured
- **THEN** `ha-user` renders, carrying its role prompt and no tooling fields

#### Scenario: The ops profile requires its credential
- **WHEN** the bundle is enabled with no admin credential configured
- **THEN** no `ha-operator` profile is rendered, regardless of MCP configuration

#### Scenario: Profiles declare no capabilities
- **WHEN** either profile is rendered
- **THEN** it contains no tool allowlist and no MCP server declaration

#### Scenario: The operator's stop is a gate, not a refusal
- **WHEN** the `ha-operator` profile is rendered with the bundle's default prompt
- **THEN** its system prompt states that a person's confirmation in the thread authorizes the described action, and that the agent does not ask again or defer the action to the person

### Requirement: Tooling is split by risk and enumerated, never wildcarded
The bundle SHALL render two `MCPToolset` objects: one covering read-only Home
Assistant operations and one covering operations that change the house. Patterns
SHALL be ENUMERATED rather than expressed as a server-wide wildcard, because a
single prefix spans both halves and would defeat the split.

The admin toolset SHALL render only when a server that registers those
operations exists.

The admin toolset SHALL ship the tools that answer the failures the
administrator exists for, INCLUDING the removal of registry objects. Removing a
stale registry entry is administration, and the operator's role prompt is the
gate on it. Registry objects are:

- entities, devices, areas and floors, zones
- helpers and integrations
- categories, labels, groups
- to-do items and calendar events

The admin toolset SHALL withhold three classes by default:

| Class | Holds |
|---|---|
| restart and backups | tools that restart Home Assistant or manage its backups |
| software and preferences | tools that install or update software, or change instance-wide preferences |
| authored configuration | tools that delete what a person WROTE — automations, scripts, scenes, dashboards, dashboard resources |

The withheld list SHALL be stated in values beside the shipped one, so adding a
tool is a values decision that restates the list.

The `MCPConfig` SHALL render only when its endpoint is configured, and its
server key SHALL be fixed by the bundle so that toolset patterns and bound
servers cannot drift apart.

#### Scenario: A wildcard cannot stand in for the split
- **WHEN** the toolsets are rendered
- **THEN** each lists specific tool patterns and neither is a single server-wide wildcard

#### Scenario: Registry removal ships
- **WHEN** the admin toolset is rendered with the bundle's default list
- **THEN** it grants `ha_remove_entity` and `ha_remove_device`

#### Scenario: Restart, backups, software and authored configuration stay withheld
- **WHEN** the admin toolset is rendered with the bundle's default list
- **THEN** it grants none of `ha_restart`, `ha_manage_backup`, `ha_manage_hacs` and `ha_config_remove_automation`

#### Scenario: No endpoint, no MCP config
- **WHEN** the MCP endpoint is left unset
- **THEN** no `MCPConfig` renders and the render succeeds
