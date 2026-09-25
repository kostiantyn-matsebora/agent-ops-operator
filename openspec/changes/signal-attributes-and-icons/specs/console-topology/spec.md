## MODIFIED Requirements

### Requirement: Pipeline topology graph
Nodes SHALL cover every CRD kind a Pipeline involves — SignalSources, SignalAdapters, Pipelines, AgentProfiles, AgentRuntimes, Channels, ChannelAdapters, MCPToolsets and MCPConfigs — not only the wiring spine. Edges SHALL cover `feeds`, `answers`, `posts`, `served-by` and `uses`.

Every node SHALL be drawn with its icon, resolved by the ladder the `resource-icons` capability states: the object's own, else its kind's built-in one.

Node health SHALL be computed exclusively from conditions reconcilers already write (`Ready`, `Served`, `Wired`).

- The console SHALL compute no health of its own, so the graph cannot disagree with `kubectl`.
- Kinds that report no health SHALL render as such, distinctly from kinds whose health is not yet known.
- Unclaimed sources and unwired channels SHALL render detached with their condition reason.
- References resolving to nothing SHALL render as broken edges to placeholder nodes rather than being omitted.

A `feeds` or `posts` edge whose ref carries a matcher SHALL be marked as conditional, and the matcher's expressions SHALL be readable from the edge.

#### Scenario: The graph agrees with kubectl
- **WHEN** a reconciler reports a failing condition
- **THEN** the node shows that condition's reason verbatim, and no node is colored by a judgement the cluster did not make

#### Scenario: Tooling is on the graph
- **WHEN** a Pipeline binds toolsets and MCP configs
- **THEN** they appear as nodes joined by `uses` edges

#### Scenario: A typo is visible
- **WHEN** `spec.adapter` names an adapter that does not exist
- **THEN** a broken edge to a placeholder node is drawn, and the Channel's `Served=False` reason is shown

#### Scenario: Healthy pipeline renders connected
- **WHEN** a Ready Pipeline wires a Served SignalSource to a profile and two Served channels
- **THEN** the graph shows the source, pipeline, profile, and both channels connected, with healthy status coloring

#### Scenario: Unclaimed source is visibly dropped
- **WHEN** a SignalSource is claimed by no Pipeline (`Wired=False`)
- **THEN** it renders as a disconnected node carrying the Wired condition's reason, making the signal-dropping state visible

#### Scenario: Unserved adapter reference is diagnosable
- **WHEN** a Channel names `spec.adapter: slak` and no such ChannelAdapter exists
- **THEN** the channel node shows `Served=False` with the condition reason, and no edge to an adapter node is drawn

#### Scenario: Every node has an icon
- **WHEN** an adopter's Channel declares no icon and the chart's Pipeline declares `aops:kubernetes`
- **THEN** the Channel node draws the built-in channel icon and the Pipeline node draws the kubernetes one

#### Scenario: A matched binding is marked
- **WHEN** a Pipeline binds `pager` with `severity In [critical]`
- **THEN** the `posts` edge to `pager` is drawn conditional, and opening it shows the expression

### Requirement: CR inventory views
The console SHALL provide per-kind inventory views listing each agentops CR with its icon, its key spec fields, conditions, and age, and a detail view showing the full object (spec and status). Opaque `config` blocks SHALL be displayed verbatim without interpretation.

#### Scenario: Condition drill-down
- **WHEN** a user opens a Channel showing `Ready=False`
- **THEN** the detail view shows the condition's reason and message as reported by the serving adapter

#### Scenario: A row carries its icon
- **WHEN** the SignalSource inventory lists a source with `spec.icon: 🔥` and one with none
- **THEN** the first row shows the emoji and the second the built-in source icon
