## Context

See `proposal.md` — Why. What already exists and shapes the approach:

- **`Graph.tsx` is shared** by the topology page and the conversation tab. Both
  problems live in that shared component, so both fixes land once.
- **`Graph` already tracks `selected`** and renders `NodeDetails` from it. The
  scope can ride on the selection the component already has.
- **`model.ts` already owns filtering AND its honesty accounting.**
  `visibleGraph()` computes health over ALL nodes before filtering and reports
  hidden failures separately, under a stated rule: a filter may simplify the
  picture but may not make a broken component disappear. A scope is another
  filter and answers to the same rule.
- **`Viewport` already re-fits on `fitKey`**, a join of the visible node ids. So
  a scope that changes the node set re-fits for free.
- **`Viewport.fit()` measures the host with `getBoundingClientRect()`** and is
  called from a `useEffect` on mount.

### The defect, precisely

PatternFly's `Tab` puts inactive tab content in the DOM with `hidden`. The
conversation graph therefore mounts into a host that measures **0 × 0**, and:

```
k = clamp(min((0 - 24) / contentWidth, (0 - 24) / contentHeight, 1))
```

Both ratios are negative, so `k` clamps to `MIN_SCALE` (0.25), and the centring
terms `(0 - contentWidth * k) / 2` come out negative. That is exactly "small and
in the top-left corner". Nothing re-runs the fit when the tab is shown, because
the node set has not changed and `fitKey` is what drives a re-fit.

## Goals / Non-Goals

**Goals:**

- One click answers "what is this connected to", at a depth the operator picks.
- A graph fits wherever it is mounted, including places that do not exist yet.
- Neither change can hide a failing element without saying so.

**Non-Goals:**

- Any BFF or Go change. The console already ships the whole graph.
- Layout changes. Scoping filters the node set and re-runs the existing layout.
- Persisting a scope, or putting it in the URL.
- Reworking the Display panel.

## Decisions

### 1. Scope is derived state over the ALREADY-VISIBLE graph

Order is `visibleGraph()` → `scopedGraph()` → `layout()`. Class hiding first,
scope second.

That order is a decision, not an accident: reachability is computed over what
the operator can SEE, so a hidden class never acts as an invisible stepping
stone joining two elements. The consequence — hiding a class can split a
component and shrink a scope — is the honest reading of "connected to what I am
looking at".

*Alternative — scope over the full topology, then hide classes.* Rejected: it
would show elements as connected through something not on screen.

### 2. The scope is the ROUTE through the node — upstream ∪ downstream

Breadth-first twice from the focused id: once following `from → to`, once
following `to → from`. A node's hop count is the smaller of the two. `depth:
'all'` keeps everything either walk reached, a numeric depth cuts by that
distance, and an edge is kept when both its ends are.

Both directions, because a scope answers "what is this part of" as much as "what
does this reach" — forward-only would scope a channel to nothing.

**Undirected traversal was tried first and was wrong.** It gives the same answer
on a small graph and a useless one on a real install, because agent-ops shares
objects deliberately: one AgentRuntime serves every profile, one Channel receives
from every pipeline. Walking without direction turns each of those into a
shortcut. Measured on a live 30-node install:

| Focused element | Undirected reaches | By route |
|---|---|---|
| `pipelines/k8s-ops` | 29 of 30 | **15**, no `ha-*` at all |
| `agentprofiles/k8s-engineer` | 29 of 30 | **6** |

The Home Assistant toolsets sat 3 hops from a Kubernetes pipeline, via the
console channel. The depth control was working perfectly and was useless, which
is why this was reported as "levelling is not working".

A five-node fixture cannot show this. Only a graph with real hubs can, and the
lesson is worth more than the fix: **a hop count is a proxy for "related" only in
a graph without hubs.**

### 3. Depth defaults to `all`, and a hub returning the whole graph is CORRECT

The default is the whole connected component, because the question "what is this
wired to" has that as its complete answer, and the narrower ones are refinements
of it.

A shared runtime or a shared channel will therefore scope to most of the
install. The view still reports itself as scoped and still names the focused
element, so the operator can tell "this connects to everything" from "the scope
did not apply" — two very different facts that would look identical if the view
quietly reset itself.

### 4. Selecting the focused element again resets

Click a node: scope to it. Click a different node inside the scope: re-scope
there, which makes drilling along a route natural. Click the focused node again:
reset. An explicit **Reset** control does the same, because a toggle nobody
discovers is not a way out.

### 5. Out-of-scope accounting extends the existing summary rather than adding a second one

`visibleGraph()` already returns a `hiddenSummary` of count, failing count and
failing classes. The scope produces the same shape for what IT removed, and the
view reports the two separately — "hidden by the display panel" and "outside the
scope" are different statements and a merged number would answer neither.

Health totals stay computed over the whole topology, untouched by either.

*Alternative — merge the two into one "not shown" figure.* Rejected: an operator
who hid a class knows they did; a scope is transient, and conflating them makes
the standing filter look like part of the transient one.

### 6. The viewport re-fits from a ResizeObserver, not from a tab callback

`fit()` becomes a no-op when the host measures zero in either axis, and records
that a fit is owed. A `ResizeObserver` on the host performs it as soon as the
host has a real size.

That fixes the tab case without knowing anything about tabs — it works for a
drawer, an accordion, a lazily revealed card, and any container invented later.

Re-fitting on every resize would fight an operator who has panned or zoomed, so
the observer only re-fits while the view is **unadjusted**. Pan and zoom set
adjusted, and the explicit fit control and a `fitKey` change clear it.

*Alternative — `mountOnEnter` on the conversation's Graph tab.* It fixes this one
page and leaves the trap set for the next place a graph is embedded. Rejected.

*Alternative — re-fit whenever the host resizes, unconditionally.* Rejected: the
operator zooms into a corner, the window resizes, and their view is thrown away.

### 7. The scope bar is its own row, above the graph

It appears only while scoped, and carries the focused element's name, the depth
control and Reset. It sits beside the existing hidden-class indicator, so
everything currently narrowing the picture is reported in one place.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Clicking to read details now also narrows the graph | It is what was asked for, and the way back is two ways: click the same element again, or Reset. The bar names the focused element so the narrowing is never silent |
| A scope hides a failing element | Decision 5 — reported and named by class, same rule class hiding already answers to |
| `ResizeObserver` fires during layout and loops | It only sets state when a fit is actually owed and the view is unadjusted, and `fit()` writes the same transform for the same size, so it converges |
| Hiding a class silently shrinks a scope | Decision 1, stated in the spec. The alternative connects elements through something off screen, which is worse |
| A route excludes something an operator expected to see | It is the price of depth meaning anything at all. The `all` default still shows the whole route, and the out-of-scope count says how much is not on it |
| The conversation graph is usually one route, so scoping it adds little | It costs nothing to have — the component is shared — and a conversation with several channels and toolsets is not trivial |

## Migration Plan

None. Console UI only, shipped in the console image. Rollback is a revert; no
state is persisted, and the BFF contract does not change.
