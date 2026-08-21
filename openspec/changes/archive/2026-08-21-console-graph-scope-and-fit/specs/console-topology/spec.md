## ADDED Requirements

### Requirement: Clicking an element scopes the graph to what it is connected to

Both the wiring graph and the conversation graph SHALL support a **scoped view**.
Selecting an element SHALL narrow the graph to that element and everything
connected to it, and SHALL leave the details panel behaving as it does unscoped.

The scope SHALL be the **route through** the element: everything upstream of it
and everything downstream of it. Both directions, so a scope answers "what is
this part of" as well as "what does this reach".

A route SHALL NOT TURN AROUND. Walking the wiring as if it were undirected makes
every SHARED element a shortcut, and this system shares elements on purpose — one
AgentRuntime serves every profile, one Channel receives from every pipeline. A
hop count is only a proxy for "related" in a graph without hubs, and this graph
is mostly hubs, so an undirected walk reports almost the whole install as
connected to almost anything.

Reachability SHALL be evaluated over the elements currently VISIBLE, so a class
hidden by the display control is not a stepping stone between two elements the
operator cannot see.

A **depth control** SHALL bound the scope in hops and SHALL default to **all**.
Scoping a heavily shared element at `all` may legitimately return most of the
graph — that is the true answer for that element, not a failure of the control,
and the view SHALL NOT special-case it.

Returning to the whole picture SHALL be possible **without reloading or
re-navigating**: an explicit reset, and selecting the focused element a second
time.

The scope SHALL NOT persist across navigation or reload. The display control's
selections persist deliberately, because they express a standing preference. A
scope expresses a question being asked right now, and a page that reopened
already narrowed would present a filtered graph as the whole one.

While a scope is in force the view SHALL name the element it is scoped to, so a
narrowed graph is never mistaken for a small installation.

#### Scenario: An operator asks what one element is wired to

- **WHEN** an element on the graph is selected
- **THEN** the graph shows that element and everything connected to it, and
  names the element it is scoped to

#### Scenario: Connection runs both ways

- **WHEN** a channel several pipelines post to is scoped
- **THEN** every pipeline that reaches it is shown, not only what it reaches

#### Scenario: A shared element is not a shortcut

- **WHEN** two pipelines post to one channel and one of them is scoped
- **THEN** the other pipeline and its own capabilities are NOT shown, because
  reaching them means arriving at the shared channel and setting off again

#### Scenario: Depth is meaningful on a real installation

- **WHEN** a pipeline is scoped on an install where one runtime serves every
  profile and one channel receives from every pipeline
- **THEN** narrowing the depth narrows what is shown, rather than every depth
  showing substantially the whole graph

#### Scenario: The scope is narrowed

- **WHEN** the depth control is reduced from all to one hop
- **THEN** only the element's immediate neighbours remain, and the count of
  connected elements beyond that depth is reported

#### Scenario: A hub element is scoped

- **WHEN** an element that nearly everything connects to is scoped at all depths
- **THEN** nearly the whole graph is shown, and it is still reported as a scope
  rather than silently behaving as a reset

#### Scenario: Returning to the whole picture

- **WHEN** the operator resets the scope, or selects the focused element again
- **THEN** the whole graph returns, with the display control's own selections
  untouched

#### Scenario: A new visit is not still filtered

- **WHEN** the operator scopes a graph, navigates away and returns, or reloads
- **THEN** the graph opens unscoped, while the display control's selections are
  restored as before

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
