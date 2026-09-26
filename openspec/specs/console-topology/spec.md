# console-topology Specification

## Purpose

The console's picture of the INSTALL: the wiring as a graph, the CR inventory
behind it, and traffic drawn on it from recorded activity events rather than
inferred from configuration.

Everything here follows from one rule — the console renders what it was told, and
never guesses. An edge is drawn because an event travelled it, a scope states
what it put out of view, and a time window bounds what is claimed rather than
letting an old event look current.

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

The topology SHALL be offered as three views of one activity feed, each a
layer of the architecture with its own node identity. A hop SHALL be mapped
onto every view's nodes, so the same event animates each view.

**Model.** The declared objects and what references what: SignalSources,
SignalAdapters, Pipelines, AgentProfiles, AgentRuntimes, Channels,
ChannelAdapters, MCPToolsets, MCPConfigs and Conversations.

Edges SHALL cover `feeds`, `answers`, `runs-on`, `posts`, `served-by`, `uses`
and `opened`. The runtime edge SHALL run from the Pipeline, never from the
profile. A runtime's image, harness and vendor SHALL be facts in its panel,
not nodes.

**Components.** One node per component the repository builds: each signal and
channel adapter, the gateway, the manager, each runtime image, context-sync,
egress-proxy and housekeeping, plus the systems outside: each model, each MCP
server, each repository, the senders an adapter declares, and the Kubernetes
API.

A runtime image running in several pods SHALL be one node carrying the count.

**Infrastructure.** Every pod, and the externals. Nothing from the model is a
node. A pod's pipeline and conversation SHALL be attributes in its panel. A
conversation pod SHALL be one node until expanded into its containers, and
SHALL collapse again.

Node health SHALL be computed exclusively from conditions reconcilers already
write, and from pod phase for pods. The console SHALL compute no health of its
own. References resolving to nothing SHALL render as broken edges to
placeholder nodes rather than being omitted.

#### Scenario: One hop, three pictures
- **WHEN** a run is dispatched
- **THEN** the Model view moves the pipeline's runtime edge, the Components view moves manager to context-sync to the runtime image, and the Infrastructure view moves the manager's pod to the conversation's pod

#### Scenario: The runtime is wired from the pipeline
- **WHEN** a Pipeline names a runtime
- **THEN** the Model view draws the edge from the Pipeline to the runtime, and no edge from the profile to a runtime

#### Scenario: A component appears once
- **WHEN** three conversation pods run the same runtime image
- **THEN** the Components view shows one runtime image node with a count of three

#### Scenario: A pod opens into its containers
- **WHEN** a conversation pod is expanded on the Infrastructure view
- **THEN** it becomes a box holding context-sync, agent and egress-proxy, the work hop reaches the agent through context-sync, every model and tool call leaves through egress-proxy, and collapsing restores the one node

#### Scenario: A typo is visible
- **WHEN** `spec.adapter` names an adapter that does not exist
- **THEN** a broken edge to a placeholder node is drawn, and the Channel's `Served=False` reason is shown

#### Scenario: Unclaimed source is visibly dropped
- **WHEN** a SignalSource is claimed by no Pipeline (`Wired=False`)
- **THEN** it renders as a disconnected node carrying the Wired condition's reason

#### Scenario: The graph agrees with kubectl
- **WHEN** a reconciler reports a failing condition
- **THEN** the node shows that condition's reason verbatim on every view that draws it, and no node is colored by a judgement the cluster did not make

#### Scenario: Tooling is on the graph
- **WHEN** a Pipeline binds toolsets and MCP configs
- **THEN** the Model view shows them as nodes joined by `uses` edges, and the Components view shows the MCP servers those configs point at

#### Scenario: Healthy pipeline renders connected
- **WHEN** a Ready Pipeline wires a Served SignalSource to a profile, a runtime and two Served channels
- **THEN** the Model view shows the source, pipeline, profile, runtime and both channels connected, with healthy status coloring

#### Scenario: Unserved adapter reference is diagnosable
- **WHEN** a Channel names `spec.adapter: slak` and no such ChannelAdapter exists
- **THEN** the channel node shows `Served=False` with the condition reason, and no edge to an adapter node is drawn

### Requirement: CR inventory views
The console SHALL provide per-kind inventory views listing each agentops CR with its key spec fields, conditions, and age, and a detail view showing the full object (spec and status). Opaque `config` blocks SHALL be displayed verbatim without interpretation.

#### Scenario: Condition drill-down
- **WHEN** a user opens a Channel showing `Ready=False`
- **THEN** the detail view shows the condition's reason and message as reported by the serving adapter

### Requirement: Traffic animates from recorded events, never from inference

Every edge with recorded events in the window SHALL carry a continuous stream
of marks whose density follows the edge's event rate, with error marks in the
edge's own proportion of errors, and unconfirmed delivery marked distinctly.

An edge with no events in the window SHALL render idle.

Every recorded hop SHALL additionally be drawn as a pulse travelling the
direction the hop travelled, distinct from the stream, within one second of
its arrival. A hop with no destination SHALL pulse on its node.

The console SHALL NOT animate an edge because a status field changed, and
SHALL NOT synthesize traffic it did not observe. The stream's density is a
rendering of the recorded rate, not an event.

#### Scenario: A quiet edge still reads as alive
- **WHEN** an edge has four events a minute in the window
- **THEN** it carries a steady stream of marks, and each of the four hops also crosses it as a pulse when it arrives

#### Scenario: Silence is visible
- **WHEN** no events reference an edge in the selected window
- **THEN** the edge renders idle rather than being hidden or animated

#### Scenario: Errors surface on the edge
- **WHEN** a fifth of an edge's events carry `status: error`
- **THEN** a fifth of its stream marks are error marks, and the edge is toned as failing

#### Scenario: A hop moves the edge it names
- **WHEN** an activity event with `from` and `to` naming two graph nodes arrives
- **THEN** that edge, and only that edge, shows a pulse within one second, in the direction the hop travelled

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

**"Each view" here is the three topology views — Model, Components,
Infrastructure — never the per-conversation graph**, which has no class
toggle and reads only the shared time window.

Each view SHALL offer its own element classes in the display control, listing
only the classes that view can draw, and SHALL remember its own hiding.

The one class a view cannot do without SHALL be listed and not hideable:
pipelines on Model, the manager on Components, the manager's pod on
Infrastructure.

A **detail** control SHALL fold the Model view only, from routes only, one
node per pipeline with its profile, runtime and capabilities inside it, to the
full model. It SHALL be absent on the other views.

The control SHALL additionally offer: traffic animation on or off, idle nodes
and idle edges shown or hidden, and edge labels selectable between none, event
rate and latency, defaulting to none. Selections SHALL persist across
navigation and reload, per view.

Hiding a class SHALL be presentation only. Health totals and the problem
rollup SHALL be unaffected, and the control SHALL say when hidden elements
include failures.

#### Scenario: Classes belong to the view
- **WHEN** the operator hides channel adapters on the Components view and switches to Model
- **THEN** the Model view's classes are untouched, and switching back finds channel adapters still hidden

#### Scenario: Routes only
- **WHEN** detail is set to routes only
- **THEN** each pipeline is one node with its profile, runtime, toolsets and MCP configs folded in, and only sources, channels and adapters remain around it

#### Scenario: A hidden failure is still reported
- **WHEN** a class containing a failing element is hidden
- **THEN** the element is still counted in the health summary and the overview problem rollup, and the display control indicates that hidden elements include failures

#### Scenario: Tooling can be collapsed away
- **WHEN** the operator hides MCP toolsets and MCP configs on the Model view
- **THEN** those nodes and their `uses` edges disappear, the remaining wiring keeps its health, and the selection survives a reload

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

### Requirement: Clicking an element scopes the graph to what it is connected to

**This is the three topology views again**, exactly as above. The
per-conversation graph is not scoped by any of these means — it is already
one conversation's own elements, with nothing to narrow it further to.

Scope SHALL be offered three ways: a **pipeline selector** showing the
selected routes only, a **scope to route** through any element with a depth
control, and **find** and **hide** expressions over the view's facts.

All three SHALL count what they put out of view and name the classes of any
failing element hidden.

A route SHALL be the pipeline and everything it reaches along the wiring, down
and up, never turning around through a shared element.

A route walk SHALL NOT pass through a channel adapter's served-by edge into a
signal source, since that is the adapter's second role. A shared runtime
image's calls SHALL be credited along the route's MCP config, not along the
image.

**Which pipeline a node belongs to is computed over the WHOLE view.** Hiding
a class SHALL NEVER disconnect a node from its route, since hiding is
presentation only.

Scoping to a route or an element, by contrast, walks only what is currently
DRAWN. A hidden class between the scoped element and something it reaches
SHALL cut the walk there.

The depth control SHALL default to all and offer only the levels the route
has. Returning to the whole picture SHALL need no reload. A scope SHALL NOT
persist across navigation. While scoped, the view SHALL name what it is scoped
to.

#### Scenario: Selecting a pipeline removes the others
- **WHEN** one of three pipelines is selected
- **THEN** the other two and everything only they reach are hidden and counted, and their routes' MCP servers are not drawn

#### Scenario: A shared adapter is not a shortcut
- **WHEN** a channel adapter also serves a chat source and one pipeline posting to that channel is scoped
- **THEN** no other pipeline is reached through the adapter and the source it serves

#### Scenario: Depth is meaningful on a real installation
- **WHEN** a pipeline is scoped on an install where one runtime serves every profile
- **THEN** narrowing the depth narrows what is shown, rather than every depth showing substantially the whole graph

#### Scenario: A new visit is not still filtered
- **WHEN** the operator scopes a graph, navigates away and returns, or reloads
- **THEN** the graph opens unscoped, while the display control's selections are restored

### Requirement: Find and hide are one grammar, over the view's own facts

A find or hide expression SHALL be terms joined by `and`, each testing one
fact of the view's own nodes: health (`healthy`, `!healthy`), activity
(`idle`, `!idle`), attachment (`detached`), or an equality/match on `kind`,
`name` (`=` exact, `~` substring), `bundle`, `node` or `pipeline`.

A term the grammar does not recognise SHALL match nothing and SHALL be named
to the operator, never silently dropped.

**Find** SHALL narrow the view to matching nodes and what reaches them.
**Hide** SHALL remove matching nodes from the view. Both SHALL count what
they put out of view exactly as the other scoping mechanisms do.

#### Scenario: An unknown term matches nothing and says so
- **WHEN** an expression contains a term the grammar has no rule for
- **THEN** it hides or finds nothing on that term's account, and the term is named as unknown

#### Scenario: Terms combine with `and`
- **WHEN** the expression is `kind=pipeline and !healthy`
- **THEN** only unhealthy pipelines match

#### Scenario: An operator asks what one element is wired to
- **WHEN** an element on the graph is selected and scoped
- **THEN** the graph shows that element and everything on its route, and names the element it is scoped to

#### Scenario: Connection runs both ways
- **WHEN** a channel several pipelines post to is scoped
- **THEN** every pipeline that reaches it is shown, not only what it reaches

#### Scenario: A shared element is not a shortcut
- **WHEN** two pipelines post to one channel and one of them is scoped
- **THEN** the other pipeline and its own capabilities are NOT shown, because reaching them means arriving at the shared channel and setting off again

#### Scenario: The scope is narrowed
- **WHEN** the depth control is reduced from all to one hop
- **THEN** only the element's immediate neighbours remain, and the count of connected elements beyond that depth is reported

#### Scenario: A hub element is scoped
- **WHEN** an element that nearly everything connects to is scoped at all depths
- **THEN** nearly the whole graph is shown, and it is still reported as a scope rather than silently behaving as a reset

#### Scenario: Returning to the whole picture
- **WHEN** the operator resets the scope, or selects the focused element again
- **THEN** the whole graph returns, with the display control's own selections untouched

### Requirement: A scope reports what it put out of view

A scope is a filter, and this graph's standing rule is that a filter may
simplify the picture but SHALL NOT conceal a broken component.

While scoped, the view SHALL report how many elements are out of scope, and
SHALL NAME the classes of any out-of-scope element that is **failing**. Health
totals and the overview problem rollup SHALL be computed over the whole graph
and SHALL be unaffected by scoping, exactly as they are by class hiding.

#### Scenario: A failure sits outside the scope

- **WHEN** a scope excludes an element whose health is bad
- **THEN** the view says so and names that element's class, and the health
  summary still counts it

#### Scenario: Scoping does not change health

- **WHEN** a graph is scoped
- **THEN** the reported health totals are the same as they were unscoped

### Requirement: A graph fits the area it is displayed in, wherever it is mounted

Every graph SHALL be scaled to fit its viewing area and centred in it when it is
first DISPLAYED — not merely when it is first mounted. A graph placed inside a
container that is in the document but not yet visible, such as an inactive tab,
SHALL fit when that container becomes visible.

It SHALL re-fit when its viewing area changes size, and SHALL STOP doing so once
the operator has panned or zoomed, so an automatic fit never overrides a view
someone chose. An explicit fit control, and a rebuild of the graph, SHALL return
it to fitting.

#### Scenario: A graph mounted in an inactive tab

- **WHEN** a graph that was rendered inside a hidden tab is shown for the first
  time
- **THEN** it is scaled to the visible area and centred, rather than left at
  minimum zoom in a corner

#### Scenario: The window is resized

- **WHEN** the viewing area changes size and the operator has not panned or
  zoomed
- **THEN** the graph re-fits to the new area

#### Scenario: A chosen view is not overridden

- **WHEN** the operator has zoomed in on part of the graph and the area is then
  resized
- **THEN** the graph keeps the operator's view, and the fit control restores
  fitting on demand

### Requirement: Layouts are a hub's, a network's and a flow's

Each view SHALL offer three layouts and remember its own: **concentric**, the
view's hub at the centre and rings by distance, each node placed near its
inner neighbour, **cola**, a constraint layout with groups kept together, and
**dagre**, ranks along the flow.

After a concentric or cola layout the picture SHALL be compacted: members
settled inside their groups, each group packed as one rectangle under a
gravity shaped to the canvas, and positions separated until no two boxes or
marks intersect.

The canvas SHALL take the picture's aspect, so the fit is bound by width.

Defaults SHALL be cola for Model and Infrastructure and concentric for
Components. A dragged node SHALL keep its new place until the next layout.

#### Scenario: A hub is drawn as a hub
- **WHEN** the Components view is laid out concentrically
- **THEN** the manager is at the centre, its adapters and sidecars on the first ring, and the externals on the rim, with no two marks or boxes intersecting

#### Scenario: The canvas is used
- **WHEN** a view is laid out with cola on a wide canvas
- **THEN** the picture fills the canvas with no empty region larger than a node, and no box overlaps another

### Requirement: Boxes are ownership, never kind

A box SHALL group elements by who owns them.

**On Model, boxing is an operator CHOICE between two criteria, never both at
once**: bundle (the bundle that installs an object, read from its Helm
labels) or route (computed as what only one pipeline reaches). A third
choice, none, draws no boxes.

On Infrastructure, the cluster node a pod runs on. On Components, no box.
Shared substrate SHALL stay unboxed under either Model criterion.

No rule SHALL position a box. A box SHALL sit where its members' edges put
it, and an element that belongs to no box SHALL never sit inside one.

#### Scenario: A bundle is a box
- **WHEN** the Model view is boxed by bundle
- **THEN** the objects one bundle installs share one box, the runtime two bundles use is unboxed, and the boxes lie wherever their edges place them

#### Scenario: A route is a box
- **WHEN** the Model view is boxed by route instead
- **THEN** each pipeline's own box holds only what no other pipeline also reaches, and an element two pipelines reach sits in neither

#### Scenario: An external is not on a node
- **WHEN** the Infrastructure view is boxed by cluster node
- **THEN** every pod sits in its node's box and no external system sits inside any node's box

### Requirement: What crossed is one click away

The view SHALL keep a feed of recent hops, and each edge's panel SHALL list
the hops that crossed it in the window.

A hop's pulse, a feed row and a history row SHALL each open the hop: its kind,
path, status, latency, and what it carried.

What a hop carried SHALL be shown from the hop's own bounded facts and from
the conversation's durable status, joined by conversation, run, op and input
id.

- A signal's title and labels.
- An input's text.
- A work unit's tools and context handle.
- A run's exit and result.
- An op's message type and body.
- An adapter's delivery report.
- A model call's model, tokens and stop reason.
- A tool call's tool and target.

The feed SHALL be identical on every view.

#### Scenario: A pulse opens its hop
- **WHEN** a pulse is clicked while it travels
- **THEN** the panel shows that hop's content, the same on any view

#### Scenario: Content comes from its durable home
- **WHEN** a run's completion hop is opened
- **THEN** the result shown is the one recorded on the conversation, not an excerpt carried by the event
