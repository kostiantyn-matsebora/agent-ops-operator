## Why

The Topology page draws the install as six lanes of columns. That is the
component-and-connector view projected onto a page, and it answers "what kind
is this" rather than "what talks to what".

Kiali answers the second question for a mesh, and the console already carries
most of Kiali's idioms: a display panel, traffic on edges, a scope through an
element, honesty counters. What it lacks is the picture.

Nothing in it reads as a network, and nothing in it shows the harness, the
model, the sidecars, the pods or the systems outside the cluster that an agent
actually spends its time talking to.

An interactive mockup settled the shape over one long working session, and it
is committed with this change as the design reference.

## What Changes

- **Three views of one activity feed**, each a layer of the architecture and
  each with its own element classes.
  - **Model**: the declared objects and what references what. The runtime is
    wired from the Pipeline, never from the profile. A runtime's image, harness
    and vendor are facts in its panel, not nodes.
  - **Components**: one node per component the repository builds, adapters,
    gateway, manager, runtime images, the two sidecars, housekeeping, plus the
    systems outside: model, MCP servers, repository, senders and the Kubernetes
    API. A runtime image running in three pods is one node with a count.
  - **Infrastructure**: every pod and the externals. Nothing from the model is
    a node. A pod's pipeline and conversation are attributes in its panel. A
    conversation pod is one icon until expanded into agent, context-sync and
    egress-proxy.
- **Detail** folds the Model view only, from routes only to the full model.
- **Kiali's layouts** — Concentric, Cola and Dagre — with a compaction pass
  that packs groups as rectangles so the canvas is used. **Boxes are
  ownership, never kind**: bundle or route on the Model, cluster node on
  Infrastructure, none on Components. No rule positions a box.
- **Scope is the pipeline.** A Pipelines selector shows the selected routes
  only, beside scope-to-route with depth and Find and Hide. A route walk never
  passes through a shared object's second role.
- **Traffic animation is continuous and rate-driven**, errors in the edge's own
  error proportion, with every recorded hop drawn as a pulse in the direction
  it travelled. Edge labels are off by default.
- **What crossed is one click away.** A live hop feed, a per-edge hop history,
  and a panel showing a hop's content: the signal's title and labels, the
  input, the work unit's tools and context handle, the run's result, the op's
  message and the adapter's report.
- **Window replay** in Kiali's shape: 1, 5, 10 or 30 minutes, 10 second
  frames, a slider, three speeds. **Conversation replay**: the hops of one
  conversation stepped in order with their real gaps.
- **Model and tool calls become hops.** The runtime reports its turns and tool
  calls with the work result, and the manager emits them as activity. Every
  view draws them.
- **Adapters declare the external systems they face** as interface metadata,
  so the Components and Infrastructure views can draw a sender without
  interpreting config.
- The unused `@patternfly/react-topology` dependency is removed.

## Capabilities

### New Capabilities

- `console-activity-replay`: replaying a past window of the activity feed
  frame by frame, and replaying one conversation hop by hop, on any view.

### Modified Capabilities

- `console-topology`: the graph becomes three views of one feed with their own
  classes, Kiali's layouts with ownership boxes, pipeline scoping, continuous
  traffic with hop pulses, a hop feed and a hop content panel. The
  pipeline-to-runtime edge replaces the profile-to-runtime edge.
- `activity-telemetry`: the runtime's model calls and tool calls are recorded
  as hops from its work report, with bounded structured facts and no content
  excerpts.
- `adapter-config-schema`: an adapter CR may declare the external systems it
  faces, as metadata beside `configSchema` and `credentialKeys`.

## Impact

**Code.**

- `platform/console/`: the topology BFF, the graph model and layout, the pages.
- `platform/manager/internal/activity/`: the new hop kinds and the `data` map.
- `platform/manager/internal/httpapi/`: the work result carries turns and tool
  calls.
- `runtimes/claude/`, `runtimes/ollama/`, `runtimes/copilot/`: each reports its
  turns.
- `platform/manager/api/v1alpha1/`: the adapter externals metadata.
- `chart/`: the console's read-only RBAC gains pods, deployments and cronjobs,
  and the adapter bundles declare their externals.

**Dependencies.** The console UI gains `dagre` and `webcola`, and drops
`@patternfly/react-topology`.

**Reference docs the change makes untrue.**

- `docs/console.md`: the Topology section describes lanes, nine kinds and the
  old scope rules.
- `docs/contracts.md`: the work report and the activity event vocabulary.
- `docs/concepts.md`: the adapter CR's interface metadata.
- `docs/cr-reference.md` and every generated block: rebuilt by the generator.
- `docs/CHANGELOG.md`: the new chart and image versions.

**Adopter site the change makes untrue.**

- `docs/console-guide.md`: the Topology tab's text and its screenshot.
- `docs/index.md`: the landing recording shows the old topology.
- `docs/getting-started.md`: names the lanes when it reaches the graph.
- `docs/assets/img/console/topology-*.png` and `docs/assets/video/`: build
  output of `npm run screenshots` and `npm run demo`.
