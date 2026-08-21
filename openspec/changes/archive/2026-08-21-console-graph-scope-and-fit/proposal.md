## Why

Two problems with the console's graphs, one a missing capability and one a defect.

**The topology answers "what is wired" but not "what is THIS wired to."** Past a
handful of pipelines the whole picture is the wrong picture: an operator asking
about one channel has to trace its edges by eye through everything else.

**The conversation graph renders tiny in the top-left corner.** It is the same
`Graph` component the topology uses, and on the topology it fits correctly. The
difference is that the conversation graph mounts inside a PatternFly `Tab` whose
content is in the DOM but `hidden` until selected, so the viewport measures a
0×0 host, clamps to minimum zoom, and never re-fits when the tab is shown.

## What Changes

- **Clicking an element SCOPES the graph to it** — the clicked element plus
  everything connected to it, transitively, in both directions. Selection and
  the details panel keep working exactly as they do now.
- **A depth control narrows that scope.** It defaults to **all**, and can be
  stepped down to a fixed number of hops. Choosing a hub element at `all` will
  legitimately show most of the graph, which is the true answer rather than a
  failure of the control.
- **Reset** returns to the whole picture, and clicking the focused element again
  does the same.
- **Scoping reports what it removed**, exactly as class hiding already does: an
  out-of-scope element that is FAILING is named rather than silently dropped.
  A scope is a filter, and this graph's standing rule is that a filter may
  simplify the picture but may never conceal a broken component.
- **The scope is NOT persisted.** The Display panel's selections survive reload
  on purpose; a scope must not, or the page opens mysteriously filtered.
- **The viewport re-fits when its host becomes measurable**, so a graph mounted
  hidden — in a tab, a drawer, an accordion — fits when it is first shown. It
  also re-fits on a genuine resize, but only while the operator has not panned
  or zoomed, so it never fights them.

## Capabilities

### New Capabilities

None. Both belong to the capability that already owns these views.

### Modified Capabilities

- `console-topology`: gains a scoped view over both graphs — what clicking an
  element does, the depth control, reset, and the accounting that keeps a scope
  from concealing a failure. Its viewport requirement gains the rule that a
  graph fits the area it is displayed in wherever it is mounted, which is what
  the conversation graph currently violates.

## Impact

| Area | Change |
|---|---|
| `console/ui/src/graph/model.ts` | a scope function over the visible graph, plus its out-of-scope accounting |
| `console/ui/src/graph/Graph.tsx` | scope state, the scope bar (focused element, depth, reset), click wiring |
| `console/ui/src/graph/Viewport.tsx` | re-fit when the host becomes measurable, and on resize while unadjusted |
| `console/ui/src/graph/Graph.test.tsx`, `model.test.ts` | scope and fit coverage |
| `docs/console.md`, `docs/console-guide.md` | the topology view's behaviour |

No Go code, no CRD, no chart. The BFF is unchanged — scoping is a view over the
graph the console already has.
