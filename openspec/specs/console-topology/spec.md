# console-topology Specification

## Purpose
TBD - created by archiving change visualize-agent-ops. Update Purpose after archive.
## Requirements
### Requirement: Configuration state from read-only Kubernetes watches
The console SHALL build its configuration state exclusively from list/watch of `agentops.dev/v1alpha1` resources (AgentProfile, AgentRuntime, Channel, ChannelAdapter, Conversation, Pipeline, SignalAdapter, SignalSource) in its own namespace using its own ServiceAccount, with no writes to any of them and no reads of Secrets or any non-agentops resource. Watches SHALL resume by resourceVersion and relist on 410 Gone so the cache converges after disconnects.

#### Scenario: State reflects a CR change without polling
- **WHEN** a Pipeline's `channels[]` is edited with kubectl
- **THEN** the console's topology updates from the watch event without any console restart or manual refresh

#### Scenario: Watch expiry recovers
- **WHEN** the API server returns 410 Gone for a stale watch
- **THEN** the console relists that kind, replaces its cache, and resumes watching without serving an error to browsers

### Requirement: Pipeline topology graph
Nodes SHALL cover every CRD kind a Pipeline involves — SignalSources, SignalAdapters, Pipelines, AgentProfiles, AgentRuntimes, Channels, ChannelAdapters, MCPToolsets and MCPConfigs — not only the wiring spine. Edges SHALL cover `feeds`, `answers`, `posts`, `served-by` and `uses`.

Node health SHALL be computed exclusively from conditions reconcilers already write (`Ready`, `Served`, `Wired`). The console SHALL compute no health of its own, so the graph cannot disagree with `kubectl`. Kinds that report no health SHALL render as such, distinctly from kinds whose health is not yet known. Unclaimed sources and unwired channels SHALL render detached with their condition reason; references resolving to nothing SHALL render as broken edges to placeholder nodes rather than being omitted.

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

### Requirement: CR inventory views
The console SHALL provide per-kind inventory views listing each agentops CR with its key spec fields, conditions, and age, and a detail view showing the full object (spec and status). Opaque `config` blocks SHALL be displayed verbatim without interpretation.

#### Scenario: Condition drill-down
- **WHEN** a user opens a Channel showing `Ready=False`
- **THEN** the detail view shows the condition's reason and message as reported by the serving adapter

### Requirement: Traffic animates from recorded events, never from inference
Edges SHALL animate only from activity events the system recorded, with animation speed reflecting observed event rate and error events marking the edge with the reported reason. The console SHALL NOT animate an edge because a status field changed, and SHALL NOT synthesize traffic it did not observe.

An edge whose op was enqueued but not confirmed by an adapter SHALL be rendered as sent-but-unconfirmed, distinctly from confirmed delivery.

#### Scenario: A hop moves the edge it names
- **WHEN** an activity event with `from` and `to` naming two graph nodes arrives
- **THEN** that edge, and only that edge, shows traffic within one second

#### Scenario: Silence is visible
- **WHEN** no events reference an edge in the selected window
- **THEN** the edge renders idle rather than being hidden or animated

#### Scenario: Errors surface on the edge
- **WHEN** an event carries `status: error`
- **THEN** the edge is marked as failing and carries the reported reason

### Requirement: The console's own SignalSource is a graph node
The console's origination SignalSource SHALL appear as a node beside other signal sources, wired to the Pipeline that claimed it, so origination is visible as configuration before use and as traffic after. An unclaimed console source SHALL render detached with its `Wired=False` reason.

#### Scenario: Pressing start lights the graph
- **WHEN** a user starts a conversation from the console
- **THEN** traffic appears on the edge from the console source to its claiming pipeline

#### Scenario: Unclaimed origination is visibly disconnected
- **WHEN** no Pipeline claims the console source
- **THEN** it renders detached, carrying the reason origination is unavailable

### Requirement: A conversation has its own graph and its own waterfall
The console SHALL render, per conversation, a topology of every element that conversation involved — its originating source and serving adapter, the pipeline it is attributed to, its AgentProfile, that profile's AgentRuntime, its runtime pod, every bound Channel with its adapter, and every MCPToolset and MCPConfig the conversation materialized — animated from that conversation's own events; and a sequence view of the same events ordered in time with per-hop latency.

The graph SHALL be built from the bindings the Conversation itself recorded, not from the Pipeline's current spec, and SHALL state when the two differ.

#### Scenario: Every involved element is present
- **WHEN** a conversation's graph is opened
- **THEN** its profile, runtime, toolsets, MCP configs, channels and adapters are all shown, and only that conversation's events animate

#### Scenario: Re-wiring does not rewrite history
- **WHEN** the Pipeline that produced a conversation has since been re-wired
- **THEN** the conversation graph shows the capabilities it actually materialized, and reports that the pipeline's current wiring differs

#### Scenario: Latency is answerable
- **WHEN** the sequence view is opened
- **THEN** each hop shows its duration, so a slow run is attributable to a specific hop

### Requirement: Graph elements are toggleable by class
Both the wiring graph and the conversation graph SHALL offer a display control that shows or hides element classes independently, at minimum: signal sources, channels, adapters (channel and signal), agent profiles, agent runtimes, MCP toolsets, MCP configs, and runtime pods. Toggling a class SHALL hide its nodes and the edges that terminate on them without disturbing the rest of the layout, and SHALL never alter reported health or hide a failing element without saying so.

The control SHALL additionally offer: traffic animation on or off, idle nodes and idle edges shown or hidden, and edge labels selectable between none, event rate and latency. Selections SHALL persist across navigation and reload.

Hiding a class SHALL be presentation only — the underlying graph, its health, and the problem rollup SHALL be unaffected.

#### Scenario: Tooling can be collapsed away
- **WHEN** the operator hides MCP toolsets and MCP configs
- **THEN** those nodes and their `uses` edges disappear, the remaining wiring keeps its layout and health, and the selection survives a reload

#### Scenario: A hidden failure is still reported
- **WHEN** a class containing a failing element is hidden
- **THEN** the element is still counted in the graph's health summary and the overview problem rollup, and the display control indicates that hidden elements include failures

#### Scenario: Edge labels are selectable
- **WHEN** the operator selects latency as the edge label
- **THEN** each edge with observed events shows its latency, and edges without events show no label rather than a zero

#### Scenario: Idle elements can be filtered out
- **WHEN** idle nodes are hidden
- **THEN** elements with no events in the selected window are removed from view, and the control reports how many were hidden

### Requirement: A time window bounds what the graph shows
The graph SHALL offer live and historical windows. Without a configured metrics backend, windows SHALL be bounded by the activity replay buffer, and the view SHALL state plainly when the requested window exceeds what the buffer retains rather than presenting a partial window as complete.

When a metrics backend is configured, longer windows SHALL be served from it as aggregates — rates, percentiles and depths — and the view SHALL indicate that the window carries aggregate data without per-item identity, so a long window is never mistaken for the exact per-hop record.

#### Scenario: Truncated history is admitted
- **WHEN** a window longer than the retained buffer is selected and no metrics backend is configured
- **THEN** the view reports how far back the data actually goes, rather than rendering an empty or partial chart as complete

#### Scenario: History extends when a backend is present
- **WHEN** a metrics backend is configured and a week-long window is selected
- **THEN** aggregates are served from it, labeled as aggregate rather than per-item

#### Scenario: Absence degrades cleanly
- **WHEN** no metrics backend is configured
- **THEN** every other view remains fully functional and only long windows are unavailable

