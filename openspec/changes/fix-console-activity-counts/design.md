## Context

Three independent display bugs in `platform/console/`, found by reading the
code (not guessed), confirmed against the running screenshots in the issue.
See `proposal.md` for the why and the full list of files touched.

Two of the three are aggregation bugs: code that counts over the wrong
population. The third is a pure omission: a field the backend already
computes and sorts by, never rendered.

A fourth symptom — the "Capacity" card and "Queues and capacity" page
reading `0/5` — was investigated. It traced to code that is already
correct.

`queues.go`'s `queues()` and the manager's own `runtimeSlots()`
(`platform/manager/internal/httpapi/status.go`) both count from the live
pod list, exactly as `.claude/rules/invariants.md` requires. No change is
proposed there.

See proposal.md's Impact section for what a reviewer should do if it still
disagrees with the inbox after this ships.

## Goals / Non-Goals

**Goals:**
- A scope's sidebar badge always equals the count the list shows when that
  scope is opened.
- A Coordinator node on the topology graph reports live activity exactly as
  a Pipeline node does, including zero.
- The Problems table on Overview shows each row's condition age.

**Non-Goals:**
- Changing how `runtimeSlots` or the manager's work-queue computation work.
  Investigated, found correct, out of scope.
- Changing what counts as `Working`/`Mine`/`Errored` for an individual
  conversation. Only the aggregate (badge) is wrong, not the per-row
  predicate. `ChatView.tsx`'s `TREE_PREDICATE` is already the correct
  definition the badge must match.
- Adding a timestamp to pod-level (`SourcePod`) or console-derived
  (`SourceDerived`) problem rows. Those have no `lastTransitionTime` to
  show, and render a placeholder rather than gaining a new timestamp
  source.

## Decisions

**Fix the badge, not the list.** `ChatView.tsx`'s `TREE_PREDICATE` already
matches what each scope's name promises (`working` = phase Working, read or
unread).

The bug is `accumulateScopeCounts` in `convapi.go` folding counts only for
unread rows. The fix moves that accumulation out from under the
`if s.Unread` branch so it runs over every conversation.

The Unread scope's own count keeps its existing unread-gated computation —
the one part of the current code that is already correct, and already
covered by a passing scenario in `console-unread`.

**Attribute Coordinator activity the same way Pipeline activity already
is.** `activityByPipeline` in `topology.go` calls `AttributePipeline` per
conversation. The fix adds the equivalent `AttributeCoordinator` lookup,
already used elsewhere in the same file for topology edges, and keys the
output map by whichever one resolves.

One function then serves both node classes instead of a second copy.
`Graph.tsx`'s badge condition (`n.cls === 'pipelines' && active > 0`) is
widened to `(n.cls === 'pipelines' || n.cls === 'coordinators') && active
> 0`.

**Keep "no data" distinct from "active zero" in the node-detail panel.**
`model.ts`'s `if (n.active || n.recent)` gate currently treats an idle
Pipeline (0/0) as "nothing to say," matching the existing idle-Pipeline
scenario.

Extending Coordinator attribution must not flip every idle Coordinator from
"no fact shown" to a rendered "0 active, 0 recent" that reads as new or
broken. The design keeps the existing gate as-is.

An idle Coordinator then renders exactly like an idle Pipeline does today —
no fact line.

That is the spec's "idle, not absent": idle is the gate omitting a zero
fact exactly as it already does for Pipelines, not a newly rendered zero.

**Render `Problem.since` with a relative-time string, with a tooltip
holding the raw timestamp.** This follows the pattern already used
elsewhere in the console for timestamps (the conversation list's "last
activity"). Rows with no `Since` (`SourcePod`, `SourceDerived`) render an
em dash.

## Risks / Trade-offs

- **[Risk]** Widening the Working/Mine/Errored badges to count read
  conversations too will make the numbers larger than operators are used
  to seeing. → This is the fix, not a regression. The badge was
  undercounting relative to its own list.
  The delta spec's new scenario pins the corrected behavior.
- **[Risk]** `AttributeCoordinator` may have different fallback/ambiguity
  behavior than `AttributePipeline` for a conversation attributable to
  neither (predates the ref, or ambiguous). → Match `AttributePipeline`'s
  existing behavior: an unattributed conversation counts toward neither
  node, exactly as today.
- **[Risk]** A relative-time renderer for `since` could crash on an empty
  string (pod/derived rows). → Guard on `p.since` being present before
  calling the formatter, falling straight to the em-dash case.

## Migration Plan

Pure display and aggregation fix. No data migration, no API shape change
beyond the new "Since" column reading an already-serialized field.

Ships in the ordinary chart/image release for `platform/console`. No
rollback concern beyond reverting the image tag.
