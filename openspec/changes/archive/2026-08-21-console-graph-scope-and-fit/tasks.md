## 1. The fit defect

- [x] 1.1 In `console/ui/src/graph/Viewport.tsx`, make `fit()` a no-op when the
      host measures zero in either axis and record that a fit is owed. Verify by
      unit test that a zero-size host produces no transform change rather than
      minimum zoom at a negative offset.
- [x] 1.2 Observe the host with a `ResizeObserver` and perform an owed fit as
      soon as it has a real size. Verify in the browser: open a conversation,
      switch to the Graph tab, and the graph is scaled to the panel and centred
      on first view.
- [x] 1.3 Track whether the operator has adjusted the view — pan or zoom sets it,
      the fit control and a `fitKey` change clear it — and re-fit on resize only
      while unadjusted. Verify by zooming in, resizing the window, and finding
      the chosen view intact, then pressing fit and finding it restored.
- [x] 1.4 Confirm the topology page is unchanged by all of the above: it fits on
      load exactly as before.

## 2. Scope in the model

- [x] 2.1 Add a scope function to `console/ui/src/graph/model.ts` taking the
      visible graph, a focused node id and a depth of a number or all, returning
      the reached nodes, the edges with both ends reached, and a summary of what
      it removed in the same shape `hiddenSummary` already uses. Verify with unit
      tests over a fixture graph.
- [x] 2.2 Walk UPSTREAM and DOWNSTREAM separately from the focused node and union
      them, taking each node's shorter distance — the ROUTE through it. Verify a
      channel scopes to the pipelines that reach it, and that a second pipeline
      sharing that channel is NOT pulled in: an undirected walk makes every
      shared element a shortcut, which on a real install put the Home Assistant
      toolsets 3 hops from a Kubernetes pipeline.
- [x] 2.3 Verify by test that health totals are identical scoped and unscoped,
      and that an out-of-scope failing element is counted and its class named.
- [x] 2.4 Verify by test that an unknown or absent focus id yields the whole
      visible graph rather than an empty one.

## 3. Scope in the view

- [x] 3.1 Hold the scope in `Graph.tsx` as focused id plus depth defaulting to
      all, applied between `visibleGraph()` and `layout()`. Verify the rendered
      node set narrows and the existing details panel still opens.
- [x] 3.2 Wire selection: clicking an element scopes to it, clicking another
      inside the scope re-scopes, clicking the focused one again resets. Verify
      each of the three by test.
- [x] 3.3 Add the scope bar — focused element name, depth control defaulting to
      all, Reset — rendered only while scoped, beside the existing hidden-class
      indicator. Verify it names the element and that Reset restores the whole
      graph.
- [x] 3.4 Report out-of-scope elements SEPARATELY from display-panel hidden ones,
      naming the classes of any that are failing. Verify both indicators can
      appear at once and read as two different statements.
- [x] 3.5 Confirm the scope is not persisted: navigate away and back, and reload.
      Verify the graph opens unscoped while the Display panel's selections are
      restored.
- [x] 3.6 Confirm the viewport re-fits when the scope changes, through the
      existing `fitKey`. Verify a scoped subgraph is centred and filled rather
      than left at the whole graph's transform.

## 4. Documentation

- [x] 4.1 Update `docs/console.md` with the scoped view — what selecting does,
      the depth default, reset, and the out-of-scope reporting. Verify it states
      the depth default and does not restate the guide's tour.
- [x] 4.2 Update the Topology panel in `docs/console-guide.md` to say the graph
      answers "what is this connected to", not only "what is wired". Verify the
      page still reads as one line per view.
- [x] 4.3 Re-run `npm run screenshots` in `console/ui` if the topology view's
      appearance changed, since the site's screenshots are build output. NOT
      NEEDED: the scope bar and its alert render only while something is
      selected, and `screenshots/capture.spec.ts` navigates to `/topology` and
      waits for a node without ever clicking one — so the captured view is
      byte-identical. Verified by reading that spec.

## 5. Verification

- [x] 5.1 Run `npm run typecheck` and `npm test` in `console/ui`. Verify both
      pass.
- [x] 5.2 Drive the built console against the screenshot fixture: scope a
      pipeline, a shared channel and a hub runtime; step the depth down and back
      to all; reset both ways. Verify each behaves as the spec's scenarios say.
- [x] 5.5 Add the outcome to `e2e/topology.spec.ts`, the file that exists for
      what jsdom cannot check: a real graph is scaled to its viewport and
      centred, a channel scopes upstream to the pipeline that posts to it, and a
      failing element outside the scope is named. Verify `npm run test:e2e`
      passes.
- [x] 5.3 Verify the conversation graph fits on first view of its tab, and that
      scoping works there too.
- [x] 5.4 Run `openspec validate console-graph-scope-and-fit --strict` and verify
      it passes.

## 6. What the live cluster found

Every one of these needed a real graph. The fixture agreed with the code.

- [x] 6.1 Scope by the ROUTE through the element rather than by undirected
      reachability. Verified live: `pipelines/k8s-ops` went from reaching 29 of
      30 nodes to 18, with no `ha-*` on it at all.
- [x] 6.2 Orient routes by FLOW, reversing an edge whose target is a signal
      adapter — the adapter feeds its source, so `served-by` is drawn against
      the flow at that end only. Verified live: `signaladapters/telegram` went
      from 2 nodes to 23. Fix the test fixture's edge direction, which had it
      the intuitive way round and is why this passed for so long.
- [x] 6.3 Offer only the levels a route actually has. Verified live:
      `pipelines/k8s-ops` shows `1, All` (12 → 18) and `signaladapters/telegram`
      shows `1, 2, 3, All` (2 → 5 → 20 → 23).
- [x] 6.4 Guard the level list against a route of length zero. A detached
      element threw `RangeError: Invalid array length` and took the view down.
      Verified by test over the unclaimed-source fixture.
- [x] 6.5 Build multi-arch and deploy to the live cluster, patching the
      ChannelAdapter CR rather than the reconciler-owned Deployment. Verified
      the pod serves the same bundle hash as the local build.
