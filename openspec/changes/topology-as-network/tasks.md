## 1. The runtime reports its turns

- [x] 1.1 Extend the work result in `platform/manager/internal/httpapi/` with a bounded `turns[]` and `toolCalls[]`, sizes capped by count and by field length, documented in `docs/contracts.md`. Verify: the httpapi unit tests accept a result with and without them and reject an oversized one
- [x] 1.2 Add `model.call` and `tool.call` kinds and the `runtime-image`, `model`, `mcp-server` and `external` node kinds to `platform/manager/internal/activity/`, plus the bounded `data` map on the event. Verify: the activity unit tests emit both kinds from a work report and refuse a `data` map over the bound
- [x] 1.3 `runtimes/claude/` reports each turn and tool call from the stream-json it already reads. Verify: `node --test` with a captured stream fixture produces the expected report
- [x] 1.4 `runtimes/ollama/` reports each turn and tool call from its own loop. Verify: `go test ./...` in the module
- [x] 1.5 `runtimes/copilot/` reports each turn and tool call from the SDK's events. Verify: `node --test` with a captured fixture
- [x] 1.6 The metrics registry gains counters and histograms for model and tool hops from the same emission. Verify: the manager's metrics test sees them observed

## 2. Adapters declare their externals

- [x] 2.1 Add `spec.externals[]` (name, kind) to `SignalAdapter` and `ChannelAdapter` in `platform/manager/api/v1alpha1/`, regenerate deepcopy and CRDs. Verify: `controller-gen` runs clean and `kubectl apply --dry-run=client -f chart/crds/` accepts the CRDs
- [x] 2.2 The chart's adapter bundles declare their externals: alertmanager, cron, k8s-events, ha, telegram, console. Verify: the chart render tests find each declaration

## 3. The console reads what the views need

- [x] 3.1 The chart grants the console read-only list/watch of pods, deployments and cronjobs in its namespace, in the same file as its existing grant. Verify: `serviceaccount-guard.py` and the chart render tests pass
- [x] 3.2 The console's cache watches pods, deployments and cronjobs beside the agentops kinds, resuming by resourceVersion and relisting on 410. Verify: the console's kube tests cover the new kinds
- [x] 3.3 The topology BFF serves the base graph with the bundle of each object from its Helm labels, the cluster node of each pod, each adapter's externals, each runtime's image and vendor facts, and the components derived from the deployments. Verify: `topology_test.go` covers each fact

## 4. Port the mockup: views and mapping

- [x] 4.1 PORT from `openspec/changes/topology-as-network/mockup/topology-network.html` the three views and the hop mapping into `platform/console/ui/src/graph/`: Model, Components and Infrastructure as functions from the base graph and a hop to that view's nodes, with per-view classes and the spine that cannot be hidden. Verify: `model.test.ts` maps one dispatch hop onto each view as the spec's first scenario states
- [x] 4.2 PORT the detail control as a fold of the Model view only, routes only through full model. Verify: a unit test folds a pipeline's profile, runtime and capabilities into it
- [x] 4.3 PORT the route walk with its two refusals: no passage through a channel adapter's served-by edge into a source, and a shared runtime image's calls credited along the route's MCP config. Verify: `model.test.ts` reproduces the console-adapter case and the shared-harness case
- [x] 4.4 PORT the expandable conversation pod on the Infrastructure view, with the hops re-routed through the sidecars when open. Verify: a unit test expands a pod and maps a model call through egress-proxy

## 5. Port the mockup: layout and boxes

- [x] 5.1 Add `dagre` and `webcola` to `platform/console/ui/package.json`, remove `@patternfly/react-topology`. Verify: `npm ci` and `npm run build` succeed in this worktree
- [x] 5.2 PORT the three layouts, concentric with wanted-angle placement, cola with groups, dagre with integer-weighted cycle breaking, each remembered per view. Verify: `layout.test.ts` lays each view out without a thrown error and with the hub at the centre for concentric
- [x] 5.3 PORT the compaction: members settled in groups, rectangles packed under canvas-shaped gravity, final direct separation, non-members evicted from boxes. Verify: a unit test asserts no two boxes and no two marks intersect after compaction on the fixture graph
- [x] 5.4 PORT ownership boxes: bundle from labels, route computed, cluster node from the pod, none on Components, nested pod boxes. Verify: a unit test boxes the fixture by each owner and finds every external outside every node box
- [x] 5.5 PORT the canvas sizing to the picture's aspect and the fit over marks and boxes, keeping the existing fit-on-first-display and stop-after-pan rules. Verify: `Viewport.test.tsx` covers the aspect and the fit

## 6. Port the mockup: traffic, feed, replay

- [x] 6.1 PORT the continuous rate-driven stream with error proportion and the recorded-hop pulses along the drawn path, pulses clickable. Verify: `Graph.test.tsx` renders a stream on an edge with events and a pulse for an arriving hop
- [x] 6.2 PORT the hop feed, the per-edge hop history and the hop content panel, joining content from the conversation's status by id. Verify: a unit test opens a completion hop and shows the run's recorded result
- [x] 6.3 PORT the pipeline selector, the scope chip with depth, and find and hide over the view's facts, all counting what they hide. Verify: unit tests for each expression form and for the hidden count
- [x] 6.4 PORT the window replay: intervals, ten second frames, slider, three speeds, buffer's edge admitted. Verify: unit tests for frame selection, speed and the buffer report
- [x] 6.5 PORT the conversation replay: hop list with offsets, step and play with compressed gaps, route dimming, entry from node, pod, hop and list. Verify: unit tests for stepping and dimming on each view
- [x] 6.6 Rebuild the Topology page and the conversation page's graph tab on the ported graph, with PatternFly controls in place of the mockup's toolbar. Verify: `npm run typecheck` and `vitest run` pass in this worktree

## 7. Unit tests

- [ ] 7.1 `cd platform/manager && go test ./...` with envtest, in this worktree's container mount
- [ ] 7.2 `cd runtimes/ollama && go test ./...`, `cd runtimes/claude && node --test`, `cd runtimes/copilot && node --test`
- [ ] 7.3 `cd platform/console && go test ./...` and `cd platform/console/ui && npm run test:coverage`
- [ ] 7.4 The chart render tests and `python3 .github/scripts/serviceaccount-guard.py`
- [ ] 7.5 `python3 .github/scripts/publication-guard.py` and `python3 .github/scripts/retired-vocabulary-guard.py` pass on this worktree, the mockup included

## 8. E2E tests

- [x] 8.1 A lane in `platform/manager/test/e2e/` runs a stub runtime that reports two turns and one tool call, and asserts the manager's activity endpoint holds the `model.call` and `tool.call` hops with the run's id. A cluster decides this: the report crosses the work contract into a real manager
- [ ] 8.2 The pack runs against this worktree's tree, built in the container and run on the host, and the new lane passes

## 9. Documentation

### 9.1 Reference docs

- [x] 9.1.1 `docs/console.md`: replace the Topology section with the three views, their classes, the layouts, the boxes, the scoping, the feed, the replay
- [x] 9.1.2 `docs/contracts.md`: the work result's turns and tool calls, and the activity vocabulary's new kinds and `data` map
- [x] 9.1.3 `docs/concepts.md`: the adapter CR's `externals` metadata beside `configSchema` and `credentialKeys`
- [x] 9.1.4 `python3 .github/scripts/docs-generate.py`, then `--check`, so `docs/cr-reference.md` and every generated block carry the new field
- [x] 9.1.5 `docs/CHANGELOG.md`: the chart, manager, console and runtime versions this ships, with the additive migration note
- [x] 9.1.6 `.claude/rules/structure.md` and `.claude/rules/terminology.md`: the topology's three views named where the console is described

### 9.2 Adopter site

- [x] 9.2.1 `docs/console-guide.md`: the Topology tab's text and its screenshot alt text describe the network views
- [x] 9.2.2 `docs/getting-started.md` and `docs/index.md`: every sentence that names the lanes or describes the old graph
- [x] 9.2.3 `cd platform/console/ui && npm run screenshots && npm run demo`, so the site's screenshots and the landing recording show the new topology
- [ ] 9.2.4 `python3 .claude/scripts/rules_compliance.py $(git ls-files '*.md')` and the docs lint in `docs/CLAUDE.md` pass
