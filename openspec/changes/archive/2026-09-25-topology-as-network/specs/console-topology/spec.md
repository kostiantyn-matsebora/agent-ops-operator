## MODIFIED Requirements

### Requirement: Pipeline topology graph

The topology SHALL be offered as three views of one activity feed, each a
layer of the architecture with its own node identity. A hop SHALL be mapped
onto every view's nodes, so the same event animates each view.

**Model.** The declared objects and what references what: SignalSources,
SignalAdapters, Pipelines, AgentProfiles, AgentRuntimes, Channels,
ChannelAdapters, MCPToolsets, MCPConfigs and Conversations. The runtime edge
SHALL run from the Pipeline, never from the profile. A runtime's image,
harness and vendor SHALL be facts in its panel, not nodes.

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

### Requirement: Graph elements are toggleable by class

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

### Requirement: Clicking an element scopes the graph to what it is connected to

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

## ADDED Requirements

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

A box SHALL group elements by who owns them: on Model, the bundle that
installs an object, read from its Helm labels, or the route, computed as what
only one pipeline reaches.

On Infrastructure, the cluster node a pod runs on. On Components, no box.
Shared substrate SHALL stay unboxed.

No rule SHALL position a box. A box SHALL sit where its members' edges put
it, and an element that belongs to no box SHALL never sit inside one.

#### Scenario: A bundle is a box
- **WHEN** the Model view is boxed by bundle
- **THEN** the objects one bundle installs share one box, the runtime two bundles use is unboxed, and the boxes lie wherever their edges place them

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
