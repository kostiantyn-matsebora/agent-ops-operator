## Purpose

The Home Assistant bundle: the log signal adapter, its MCP tooling, two
profiles and the wiring for both.

The split by PRIVILEGE is the whole design. Two lanes — an everyday one and
an acting one — with tooling enumerated per lane and never wildcarded, so
the everyday agent cannot reach the admin path that lets the acting one
repair the house.

Both routes render their own identity, both claim the install's chat
sources, and the acting one additionally claims the bundle's log source.
Wiring is behind a flag that defaults off, and no turnkey mode turns it on.

## MODIFIED Requirements

### Requirement: The bundle ships its wiring behind a flag that defaults off
The bundle SHALL render two `Pipeline` objects under a wiring flag defaulting to
**false**, so that enabling the bundle for its adapter, tooling and profiles
never silently adds a route beside the ones the install declared.

Nothing forces the flag on. No turnkey mode enables this bundle at all, so
the flag is a plain boolean rather than a nullable one:

- **`ha-control`** — profile `ha-user`, claiming the chat sources named in
  values, delivering to the channels named in values, binding the read-only
  toolset.
- **`ha-ops`** — profile `ha-operator`, claiming the bundle's own signal source,
  delivering to the same channels, binding both toolsets.

Turning the flag off SHALL render neither Pipeline and SHALL leave every other
component intact, so an install that declares its wiring at the parent scope can
still use the bundle.

A Pipeline SHALL render only when its profile renders. Chat source and channel
references SHALL come from values and SHALL be omitted when unset, so the bundle
never names an object another component did not create.

Under `global.agentops.wiringMode: coordinator`, the bundle SHALL render the
SAME two route identities as two standalone `AgentCapability` objects,
never the two inline Pipelines.

One capability carries `ha-user` at the everyday privilege level. The other
carries `ha-operator` at the acting level.

The bundle SHALL NOT render either `Pipeline` in this mode. The two
capabilities SHALL NEVER be merged into one — the privilege split this
bundle's whole design rests on carries over unchanged.

Nothing forces this bundle's wiring on in coordinator mode either, matching
the `pipelines`-mode rule that no turnkey mode enables this bundle at all.

#### Scenario: Enabling the bundle alone adds no route
- **WHEN** the bundle is enabled with credentials but the wiring flag is not set
- **THEN** no `Pipeline` renders, and the install's own declarations remain the only routes

#### Scenario: Asking for the wiring produces a working lane
- **WHEN** the bundle is enabled with credentials, surface names and the wiring flag set
- **THEN** both Pipelines render, each naming its profile, and the signal source is claimed rather than left inert

#### Scenario: Wiring can be declined
- **WHEN** the wiring flag is set false
- **THEN** no `Pipeline` renders, every other component still renders, and the source reports `Wired=False` until the install claims it

#### Scenario: Absent surfaces are omitted, not named
- **WHEN** no telegram channel name is supplied
- **THEN** neither Pipeline references a telegram channel or source, and both render valid

#### Scenario: No profile, no pipeline
- **WHEN** the admin credential is absent so `ha-operator` does not render
- **THEN** the `ha-ops` Pipeline is not rendered either

#### Scenario: Coordinator mode renders two capabilities, never merged

- **WHEN** `global.agentops.wiringMode: coordinator` is set and the bundle's
  wiring is enabled with both credentials configured
- **THEN** the bundle renders both the `ha-user` and `ha-operator`
  `AgentCapability` objects, each at its own privilege level, and renders
  no `Pipeline` of its own

#### Scenario: Coordinator mode's capabilities are claimed by the chart Coordinator

- **WHEN** coordinator mode is set and the bundle's wiring is enabled
- **THEN** the chart-rendered Coordinator's `agents[]` lists both of this
  bundle's `AgentCapability` objects as separate entries, and its
  `signalSourceRefs` claims the bundle's log source — so the source is not
  left `Wired=False`

#### Scenario: No turnkey mode forces this bundle's wiring on, in either posture

- **WHEN** `global.demo.enabled=true` and `global.agentops.wiringMode:
  coordinator` are both set with the bundle otherwise at defaults
- **THEN** the bundle's wiring stays off and no `AgentCapability` or
  `Pipeline` renders, exactly as under `pipelines` posture

#### Scenario: A missing admin credential still withholds the acting capability

- **WHEN** coordinator mode is set, wiring is enabled, and the admin
  credential is absent so `ha-operator` does not render
- **THEN** no `AgentCapability` for the acting route renders either, and the
  Coordinator's `agents[]` lists only the everyday one
