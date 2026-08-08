# pipeline-model — delta

## ADDED Requirements

### Requirement: Chart-managed wiring is declared once, at the top
A chart that ships components as subcharts SHALL NOT render `Pipeline` objects
from any subchart. Wiring names a profile, signal sources and channels that
routinely originate in DIFFERENT components, and a subchart can see only itself
— so a component that shipped wiring could only wire its own lane, producing one
Pipeline per lane where the install wanted one route.

Wiring SHALL instead be declared at the parent scope, which is the only one that
sees every component. A declaration SHALL require a profile and MAY name signal
sources, channels, toolsets and MCP configs; a declaration naming no profile
SHALL fail the render, because a Pipeline with no profile has no agent to run.

#### Scenario: One route spanning several components
- **WHEN** an install combines a cluster-events source from one component with a
  chat surface from another, answered by one agent
- **THEN** a single Pipeline declared at the parent scope claims both sources
  and delivers to the channel, and no component renders wiring of its own

#### Scenario: A component's source is inert until claimed
- **WHEN** a component renders a signal source and the install declares no
  Pipeline claiming it
- **THEN** the source reports `Wired=False` and drops its signals, exactly as an
  unclaimed source always does

#### Scenario: A profile-less declaration is refused
- **WHEN** a wiring entry omits its profile
- **THEN** the render fails naming the entry

### Requirement: Pipelines are named for their purpose, not their transport
A `Channel` is shareable across Pipelines by design — one chat surface carries
many jobs, so that operators need not run a bot and a group per route. A
`SignalSource` is claimed by exactly one Pipeline. Naming SHALL follow that
asymmetry: a Pipeline is named for the JOB it does, never for the channel it
answers on, because the channel will carry other jobs.

#### Scenario: Several pipelines share one chat surface
- **WHEN** two Pipelines with different purposes both deliver to one Channel
- **THEN** both are valid, and each carries its own profile and capabilities

#### Scenario: A chat source has exactly one answerer
- **WHEN** two Pipelines claim the same chat SignalSource
- **THEN** the older claimant wins and the other reports a source conflict, so
  "who answers this surface" always has one answer
