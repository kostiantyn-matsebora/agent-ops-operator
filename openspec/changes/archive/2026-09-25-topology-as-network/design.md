## Context

See proposal.md for why. What shapes the approach:

- The console draws the graph itself, in SVG, from a BFF response of nodes and
  edges. Layout is a lane placer in `graph/layout.ts`. Traffic is applied
  server-side to drawn edges and flashed client-side from the stream.
- The activity event carries ids and a `detail` string, never content. The
  content is in the conversation's status, which the console already streams.
- The manager never sees a model call or a tool call. The runtime does, and so
  does the egress proxy.
- The console watches the agentops kinds only, with read-only RBAC the chart
  renders.

## The prototype

`openspec/changes/topology-as-network/mockup/topology-network.html` is the
mockup the approach was settled against. It runs on synthetic traffic in the
page and is referenced by path, never by a link.

**The split it creates.** The mockup is the reference for COMPOSITION: the
node marks, their size against spacing, the box padding, the rings of the
concentric layout, the packing margins, the pulse sizes, the frame timings,
the panel layouts and every label.

The specs are the reference for BEHAVIOUR. The numbers the mockup settles are
written down nowhere else, on purpose.

**Where the mockup is deliberately not the target.**

| In the mockup | In the console |
|---|---|
| synthetic hops generated in the page | the manager's activity stream and buffer |
| bundles typed in a table | the Helm labels on each object |
| the pod's cluster node typed in a table | the pod's `spec.nodeName` |
| externals typed per adapter | the adapter CR's declared externals |
| hop content generated with the hop | joined from the conversation's status by id |
| one page, three views, own toolbar | the console's pages, theme and PatternFly controls |
| a Pipelines chip menu | a PatternFly select, persisted like the display panel |
| WebCola and dagre from a CDN | the same two libraries, from the package |

## Goals / Non-Goals

**Goals:**

- Port the mockup's composition, not re-derive it.
- Keep every honesty rule the graph already has: whole-graph health, hidden
  and out-of-scope counts with failing classes named, traffic only from
  recorded hops.
- Make the harness, the model, the sidecars, the pods and the externals first
  class, with the hops that touch them.

**Non-Goals:**

- Per-host connection counts from the egress proxy. A later change.
- Remembering node positions across reloads. The layout is recomputed, as it
  is in Kiali.
- Replay beyond the activity buffer. A window longer than the buffer holds is
  reported as such, as the page already does.
- Editing anything. The console stays read-only.

## Decisions

### The renderer stays the console's own SVG. `react-topology` is dropped

The dependency is declared and never imported. Adopting it now would mean
re-implementing the mockup's marks, boxes, pulses along paths and the
compaction pass inside another library's model, for layouts the mockup already
runs directly on `dagre` and `webcola`, the engines Kiali's own options use.

The mockup is proven. Porting it is the smaller and safer change. Removing the
unused dependency closes a half-made decision rather than leaving it open.

### Three views are three node identities over one feed

A view is a function from the base graph and a hop to that view's nodes. The
hop feed, the payload panel and the replay are identical across views. Only
the identity changes.

This is Kiali's graph type, and it is what keeps one event vocabulary serving
three pictures.

- Model: identity.
- Components: adapter CRs stay, model objects fold into the manager, runtimes
  and harnesses fold into the runtime image, pods fold into the component
  they run.
- Infrastructure: model objects fold into the manager's pod, runtimes and
  harnesses into the conversation's pod, or its agent container when the pod
  is expanded.

### Layout is Kiali's, plus compaction

Concentric for a hub, Cola with groups, Dagre for a flow. Each view remembers
its own.

A compaction pass follows Cola and Concentric: members settle inside groups,
then each group is one rigid rectangle packed under a gravity shaped to the
canvas, then positions are separated directly until nothing intersects.

Rectangles pack without holes, which is what circles could not do.

The canvas takes the picture's aspect after each layout, so the fit is bound
by width. Dagre is left alone.

### Boxes are ownership

Bundle, read from the Helm labels. Route, computed as what only one pipeline
reaches. Cluster node, from the pod spec. Never kind, and nothing positions a
box but its members' edges.

### The runtime reports its turns, the manager emits them

The work report gains a bounded list of turns and tool calls: model, tokens in
and out, cache reads, stop reason, tool name, target server, duration, result
size.

The manager emits `model.call` and `tool.call` hops from it, with the runtime
image as `from` and the model or MCP server as `to`.

The egress proxy is the vendor-neutral alternative, counting every host. It is
deferred: the report is precise per turn and needs no new component. The two
can coexist later.

### Hop content is joined in the browser

The event keeps ids and a `detail`. It gains a small structured `data` map,
bounded in size, for facts that exist nowhere else: tokens, tool, stop reason,
checkpoint generation and bytes.

Everything with a durable home, the input text, the run's result, the op's
message, is joined from the conversation's status by conversation, run, op and
input id. The buffer stays lossy and bounded, and no content enters telemetry.

### Externals are adapter metadata

An adapter CR declares the systems it faces beside `configSchema` and
`credentialKeys`: a name and a kind. The console draws them and interprets no
config. The bundles declare their own.

### A pod is boxed by infrastructure or not at all

On Infrastructure a pod's only box is its cluster node. Its pipeline and
conversation are attributes in its panel. On Components there are no boxes. On
Model there are no pods.

### Route walks refuse a shared object's second role

The console adapter also serves the console chat source. A route walk that
entered it through that source and left through its channel joined every
pipeline to every other.

The walk skips a channel adapter's served-by edge into a signal source. A
shared harness's calls are credited along the route's MCP config, not along
the harness.

## Risks / Trade-offs

- [Cola output plus compaction is not pure Cola, and node positions differ on
  every reload] → accepted, as in Kiali. Positions are not remembered.
- [Reading pods, deployments and cronjobs widens the console's RBAC] → read
  only, its own namespace, rendered by the chart beside the existing grant,
  and the ServiceAccount guard still fails a render that grants what nothing
  binds.
- [The work report grows] → bounded by count and by field size, documented in
  contracts.md, and a runtime that reports nothing still draws as today.
- [Three runtimes must each report turns] → claude and copilot have the
  events in hand, ollama owns its loop. A runtime that does not report simply
  has no model hops, and the panel says so.
- [Externals declared wrongly draw a wrong picture] → the declaration is
  metadata the console shows verbatim, and the bundles are reviewed with the
  adapters they ship.

## Migration Plan

Additive. An older runtime reports no turns and the graph draws none. An
adapter CR with no externals draws no senders. The display panel's persisted
selections are re-keyed per view, so an old selection is dropped rather than
misapplied.

## Open Questions

- Whether the fit on first display should show the whole picture or the hub's
  first ring. Kiali fits the whole picture. Either can be changed after the
  port without touching a spec.
