## Why

The console's inbox sidebar and topology both report activity counts an
operator cannot trust. A scope's badge disagrees with the list it opens to.
A Coordinator's running conversations never register anywhere on the graph,
no matter how many are `Working`.

Both are the same class of bug: an aggregation counting the wrong
population, found by reading the console's own aggregation code
(`platform/console/convapi.go`, `platform/console/topology.go`) against what
the corresponding list filter and graph badge actually show.

The Overview page's Problems table also drops a timestamp its own data
already carries and already sorts by, so "what changed, how long ago" is a
field the operator has to click into every row to get.

## What Changes

- **Inbox scope badges match their own list.** `accumulateScopeCounts` in
  `convapi.go` only folds UNREAD conversations into the Working/Mine/Errored
  badges, while clicking into any of those scopes filters on phase/mine/error
  state alone (`ChatView.tsx`'s `TREE_PREDICATE`), read or unread. The badge
  undercounts relative to its own list. Working/Mine/Errored badges are
  computed over the same population their scope's filter admits. Only the
  Unread badge keeps its unread-only count, since unread IS that scope.
- **Coordinator nodes carry live activity badges, same as Pipeline nodes.**
  `activityByPipeline` in `topology.go` attributes every conversation through
  `AttributePipeline` alone and never through `AttributeCoordinator`, so a
  Coordinator's `active`/`recent` counts are always zero. `Graph.tsx`
  additionally hard-excludes any node whose class is not `pipelines` from
  drawing the badge at all, so even a nonzero count would never render. A
  Coordinator with running conversations now shows the same live count a
  Pipeline does.
- **The Problems table shows when each problem started.** `Problem.Since`
  (the condition's `lastTransitionTime`) is already populated server-side
  for condition-backed rows and already drives the rollup's sort order
  (`overview.go`'s `problems()`). The `ProblemsCard` table in `Overview.tsx`
  simply never renders it. A "Since" column is added, showing it where
  present and a placeholder for the pod- and console-derived rows that carry
  no condition timestamp.
- **Investigated and found NOT to be a console bug:** the literal "Capacity"
  card and the "Queues and capacity" page (`queues.go`, `Queues.tsx`,
  `Overview.tsx`) relay the manager's own `GET /status` `runtimeSlots`
  verbatim. That already counts capacity from the live pod list exactly as
  `.claude/rules/invariants.md` requires ("active" is pod-backed, never
  status-derived). The work-queue rows are built over every non-closed
  conversation with no pipeline- or coordinator-scoped filtering to go wrong.
  No change is proposed to that code path. If it still reads inconsistently
  with the inbox's `Working` badges after this change ships, that points at
  the manager's conversation-phase reconciliation, not the console, and is a
  separate investigation.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `console-live-runs`: the "Activity badges on the topology" requirement now
  covers Coordinator nodes, not only Pipeline nodes. The "Conversations are
  filterable and paginated server-side" requirement gains a general
  scope-count-matches-its-list rule, generalizing the unread-only guarantee
  that requirement already states.
- `console-application`: the "overview page reports the installation and
  what is wrong with it" requirement now requires each problem row to carry
  its condition's transition time where one exists.

## Impact

- **Code:** `platform/console/convapi.go` (`accumulateScopeCounts`),
  `platform/console/ui/src/pages/chat/ChatView.tsx` (no change expected —
  it is already the correct behavior the badge must match),
  `platform/console/topology.go` (`activityByPipeline`),
  `platform/console/ui/src/graph/Graph.tsx` (badge render condition),
  `platform/console/ui/src/graph/views/model.ts` (node-detail active/recent
  fact, currently gated to omit a true zero — needs to keep distinguishing
  "no data" from "zero" once Coordinators report real counts),
  `platform/console/overview.go` (`Problem` already carries `Since`, no Go
  change expected), `platform/console/ui/src/pages/Overview.tsx`
  (`ProblemsCard` table), `platform/console/ui/src/api/types.ts` (`Problem`
  type already carries `since?`).
- **Reference docs:** `docs/console.md` describes the console's internals,
  including topology aggregation and overview endpoints. Update it if it
  documents the per-pipeline-only activity attribution or the Problems
  table's columns. `docs/concepts.md` and `docs/contracts.md` are
  unaffected — no CRD field, adapter contract or HTTP endpoint changes.
- **Adopter site:** `docs/console-guide.md` describes what the console's
  views answer for an adopter. Update its description of the topology's
  activity badges and the Problems table if it enumerates their columns or
  scope. No other adopter-facing page (landing, Introduction, Getting
  started, Installation) makes a claim this change makes untrue.
- **Tests:** `platform/console`'s Go tests around `accumulateScopeCounts`
  and `activityByPipeline`, and the console UI's component/unit tests around
  `Graph.tsx` badge rendering and `Overview.tsx`'s `ProblemsCard`.
