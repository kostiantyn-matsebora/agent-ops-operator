# ha-bundle

## ADDED Requirements

### Requirement: The HA bundle ships as a self-gated subchart, off by default
A Helm subchart at `chart/charts/ha-bundle/` SHALL package the Home Assistant
agent experience. Every template SHALL gate on the bundle being active, with
`ha-bundle.enabled` defaulting to `false`, so a default install renders nothing
from it.

The bundle SHALL NOT render the agent execution substrate. The `AgentRuntime`,
the runtime ServiceAccount, the LLM credential Secret and that SA's RBAC belong
to the parent chart; the bundle references them and renders none of them.

The bundle SHALL create no Secret. Every credential SHALL be referenced by name
and reach a pod through `valueFrom`/`envFrom`, never read by the manager.

#### Scenario: Default install renders nothing
- **WHEN** the chart renders with default values
- **THEN** no ha-bundle object appears, while the parent's `AgentRuntime` still renders because it is not the bundle's

#### Scenario: No substrate, no secrets
- **WHEN** the bundle is enabled with every component on
- **THEN** the output contains no `AgentRuntime`, no runtime ServiceAccount and no `Secret` of any kind

### Requirement: Components gate independently and partial installs stay valid
The ingest lane, the MCP configuration, the toolsets, each profile and the
wiring SHALL each be independently controllable, and a cross-component reference
SHALL be values-resolvable so that partial enablement renders valid objects
rather than dangling references. A reference whose target is not being rendered
SHALL be omitted, not emitted as a name that resolves to nothing.

#### Scenario: Ingest-only install
- **WHEN** the bundle is enabled with both profiles disabled
- **THEN** the adapter and its `SignalSource` render, no `AgentProfile` renders, and no `Pipeline` references a profile that does not exist

#### Scenario: Chat-only install
- **WHEN** the bundle is enabled with the ingest lane disabled
- **THEN** the profiles, toolsets and MCP configuration render, and no wiring references `ha-logs`

### Requirement: Two profiles split the house by privilege
The bundle SHALL render two `AgentProfile` objects with distinct jobs:

- **`ha-user`** — the user of the house. It SHALL render when EITHER the MCP
  endpoint OR the read-scoped API credential is configured, and SHALL NOT render
  when neither is.
- **`ha-ops`** — the administrator. The admin API credential SHALL be its
  prerequisite: it SHALL render only when that credential is configured, and MCP
  configuration SHALL be optional for it.

Each profile SHALL carry identity only — role prompt, connectivity env,
turn limit, optional runtime reference. Neither SHALL declare tools or MCP
servers, because an `AgentProfile` carries no capabilities; what each agent may
do comes from the Pipeline routing it.

Each profile SHALL carry an inline role prompt, because neither has a repository
to hold an agent definition.

#### Scenario: No configuration renders no user profile
- **WHEN** the bundle is enabled with neither an MCP endpoint nor a read credential
- **THEN** no `ha-user` profile is rendered

#### Scenario: MCP alone is enough for the user profile
- **WHEN** only the MCP endpoint is configured
- **THEN** `ha-user` renders, carrying its role prompt and no tooling fields

#### Scenario: The ops profile requires its credential
- **WHEN** the bundle is enabled with no admin credential configured
- **THEN** no `ha-ops` profile is rendered, regardless of MCP configuration

#### Scenario: Profiles declare no capabilities
- **WHEN** either profile is rendered
- **THEN** it contains no tool allowlist and no MCP server declaration

### Requirement: Tooling is split by risk and enumerated, never wildcarded
The bundle SHALL render two `MCPToolset` objects: one covering read-only Home
Assistant operations and one covering operations that change the house. Patterns
SHALL be ENUMERATED rather than expressed as a server-wide wildcard, because a
single prefix spans both halves and would defeat the split.

The admin toolset SHALL render only when a server that registers those
operations exists.

The `MCPConfig` SHALL render only when its endpoint is configured, and its
server key SHALL be fixed by the bundle so that toolset patterns and bound
servers cannot drift apart.

#### Scenario: A wildcard cannot stand in for the split
- **WHEN** the toolsets are rendered
- **THEN** each lists specific tool patterns and neither is a single server-wide wildcard

#### Scenario: No endpoint, no MCP config
- **WHEN** the MCP endpoint is left unset
- **THEN** no `MCPConfig` renders and the render succeeds

### Requirement: The bundle ships its wiring behind a flag that defaults on
The bundle SHALL render two `Pipeline` objects under a wiring flag defaulting to
**true**, so that enabling the bundle produces a working install rather than
components that look installed and drop everything:

- **`ha-control`** — profile `ha-user`, claiming the chat sources named in
  values, delivering to the channels named in values, binding the read-only
  toolset.
- **`ha-ops`** — profile `ha-ops`, claiming the bundle's own signal source,
  delivering to the same channels, binding both toolsets.

Turning the flag off SHALL render neither Pipeline and SHALL leave every other
component intact, so an install that declares its wiring at the parent scope can
still use the bundle.

A Pipeline SHALL render only when its profile renders. Chat source and channel
references SHALL come from values and SHALL be omitted when unset, so the bundle
never names an object another component did not create.

#### Scenario: Enabling the bundle produces a working lane
- **WHEN** the bundle is enabled with credentials and surface names supplied
- **THEN** both Pipelines render, each naming its profile, and the signal source is claimed rather than left inert

#### Scenario: Wiring can be declined
- **WHEN** the wiring flag is set false
- **THEN** no `Pipeline` renders, every other component still renders, and the source reports `Wired=False` until the install claims it

#### Scenario: Absent surfaces are omitted, not named
- **WHEN** no telegram channel name is supplied
- **THEN** neither Pipeline references a telegram channel or source, and both render valid

#### Scenario: No profile, no pipeline
- **WHEN** the admin credential is absent so `ha-ops` does not render
- **THEN** the `ha-ops` Pipeline is not rendered either

### Requirement: The two lanes share one surface, and only one of them claims it
Addressing a Pipeline by name in chat SHALL route to that Pipeline regardless of
which Pipeline claims the surface's source, and regardless of the addressed
Pipeline's own conditions. Claiming a source and being addressable are
INDEPENDENT: a claim decides who answers an UNADDRESSED message, and nothing
else.

Because of that independence, `ha-ops` SHALL list the bundle's own signal source
and SHALL NOT list any chat source. Listing one would grant nothing — the
command path never consults the claim — while costing the Pipeline a permanent
`SourceConflict`, since a source listed by an older Pipeline puts every later
claimant at `Ready=False`. That state SHALL be avoided not because it would stop
the agent answering, but because an unready Pipeline is omitted from the chat
listing of available agents and reads as broken everywhere it is displayed.

`ha-control` SHALL therefore be the sole claimant of the chat sources and the
default answerer, and the administrator SHALL be reached by name. Escalation
SHALL be an explicit act rather than the default behaviour of the surface.

#### Scenario: One claimant, both addressable
- **WHEN** both Pipelines render against one console surface
- **THEN** only `ha-control` lists the chat source, both Pipelines report `Ready=True`, and neither reports a source conflict

#### Scenario: Escalation is explicit and lands where it was asked
- **WHEN** a person addresses the administrator pipeline by name from a surface claimed by `ha-control`
- **THEN** the conversation runs with the administrator's profile and capabilities, and its replies arrive in the thread on the surface the request came from

#### Scenario: An unready pipeline is still addressable but no longer discoverable
- **WHEN** a Pipeline is at `Ready=False` because it lists a source an older Pipeline already lists
- **THEN** addressing it by name still opens a conversation, and it is absent from the chat listing of available agents — which is the cost the bundle's wiring avoids

### Requirement: The credential path's reach is documented, not implied
The bundle SHALL document that an API credential in a profile's environment,
combined with a shell tool bound by a Pipeline, reaches Home Assistant
regardless of what the toolsets allow. The risk split SHALL be described as a
real boundary for the MCP path and an advisory one for the credential path.

#### Scenario: The asymmetry is stated
- **WHEN** an operator reads the bundle documentation
- **THEN** it states that toolsets constrain the MCP path and that a credential plus a shell tool is a second, wider path
