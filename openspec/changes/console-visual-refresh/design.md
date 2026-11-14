## Context

See `proposal.md` for the motivation. The facts that shape the approach:

- The console UI is React with PatternFly. It lives in
  `platform/console/ui/src/`.
- Logic and presentation are tangled. `pages/chat/ThreadPane.tsx` is 998 lines
  and imports the API hooks, the stream, local state and PatternFly together.
  The chat folder carries about 120 inline `style={{` blocks.
- "Theme" already means light, dark or automatic. It is `theme/useTheme.ts`,
  `ThemeSwitcher`, the `data-theme` attribute and the storage key
  `agentops-console-theme`. The docs site copies the `--ao-*` tokens.
- The console service reads install settings from `Channel.spec.config`, such as
  `metricsUrl`, and returns facts to the browser through `GET /api/session`.
- Every `e2e-live` spec asserts today's DOM.

The exploration produced a clickable mockup at `mockup/console.html`. It is
the reference for composition. The specs are the reference for behaviour.

## Goals / Non-Goals

**Goals:**

- Two looks, `classic` and `workspace`, switchable at runtime, the new one the
  default.
- A seam that makes a third look a folder.
- The new look meets every guardrail the exploration produced, each with a
  check that can fail.
- Every look proven at phone, tablet and desktop viewports.

**Non-Goals:**

- Retiring `classic`. That is a later decision.
- Any manager, CRD or channel-contract change.
- A per-user look stored on the server.
- Redesigning what a page says. Content stays, drawing changes.

## Decisions

### D1. The new axis is a look, not a theme

| Option | Verdict |
|---|---|
| Call it a look and keep "theme" for light, dark, auto | **Chosen.** Nothing existing is renamed and the docs-site token copy is untouched |
| Rename the existing control "colour scheme" and take "theme" | Rejected. It touches the storage key, the docs-site copy and `palette-and-mark.md` for a word |

The user asked for themes. The switcher label says "Look" and the docs say a look
is the console's theme in the wider sense. A later rename stays open.

### D2. Three layers and one rule

```
api/        data, stream, client        (exists)
model/      view models: hooks, no JSX  (new)
looks/      classic/  workspace/  shared/
```

1. `model/` returns plain data and callbacks. It imports no component library, no
   stylesheet and no look.
2. A look imports `model/` and `api/types`. It never imports `api/client`.
3. A look never imports another look.
4. `looks/shared/` holds the look switcher and nothing look-specific.

A unit test walks the import graph and fails naming the file.

**Alternatives considered:**

- **A CSS skin over one component tree.** Rejected. The new layout changes the
  structure, not only the paint. Signals, folded run details, the row shape and
  the date line need different markup.
- **A fork of the app.** Rejected. Two copies of the logic drift the first week.
- **Headless components with render props.** Rejected. Hooks give the same
  separation with less ceremony in this codebase.

### D3. Move classic first, change nothing

Phase one moves today's `pages/` and `components/` into `looks/classic/` with
no behaviour change. The existing unit tests are the safety net and must stay
green with only import paths edited.

Model hooks are extracted as the workspace needs them, one view model at a time.
`classic` is moved onto each as it lands. A model that only one look uses is a
smell and is merged back.

### D4. The workspace owns its markup and uses PatternFly only for primitives

The workspace draws its own surfaces with plain CSS in
`looks/workspace/workspace.css`. It may use PatternFly for dialogs, menus and
tooltips, where accessibility work is heavy, and styles them with tokens.

Looks load lazily. A viewer downloads only the look in force.

### D5. Selection

- A zustand store `useLookStore`, persisted under `agentops-console-look`,
  beside the colour-scheme store.
- Resolution order: `?look=`, stored choice, the session's `defaultLook`, the
  build default.
- The root carries `data-look`, next to `data-theme`.
- The look switcher is part of the page contract. Every look's header renders
  it, and a conformance check fails a look that does not.

### D6. Tokens

The shared `--ao-*` tokens stay where they are. The workspace adds `--wk-*`
tokens in its own stylesheet for what the mockup needed and the shared set lacks.

The docs-site copy rule in `palette-and-mark.md` keeps applying to `--ao-*`
alone. A look token is not copied to the site.

### D7. Asynchronous state lives in the model

A `useCommands` model tracks each of the viewer's actions by target, with the
states sending, accepted, done and failed. It derives them from the response and
from the event that confirms the action.

It also exposes the in-flight count and the connection state.

Retry is offered only for a verb that is safe to repeat.

The `accepted` state depends on `async-command-bus`. Where a verb is still
synchronous the model goes from sending to done, and the look shows it without
change.

### D8. Run detail is shaped in the model

The model exposes a working run as `{ elapsed, latest, count }`. When the reply
lands it attaches the run's tools and events to that reply.

`classic` keeps rendering events inline as `console-thread-live-cues` says. The
`workspace` look folds them. Both read the same data, so neither parses events
itself.

### D9. The guardrails are checks

| Guardrail | Verified by |
|---|---|
| Nothing changes size while it animates | a Playwright run observes `layout-shift` entries and polls the header, the tab bar and every row |
| Incoming events appear in place | a spec asserts no banner or toast on a signal arrival |
| A run ends in the folded layout | a spec asserts no per-event line after a reply, for both a reply the viewer sent and a signal-started run |
| Severity is an icon | a unit test renders each contract value and asserts no chip |
| Run detail is folded | a spec asserts the collapsed `Run details` line and the visible `Invoked` row |
| No empty or duplicated rows | a unit test renders a reply and asserts no row repeats another's fact |
| Reduced motion | a spec sets `prefers-reduced-motion` and asserts no animation runs |
| Fills the viewport | a spec at 2560 by 1300 asserts the panes reach both edges and the page does not scroll |
| Every message copies | a spec reads the clipboard after copy on a reply, a person's message and a signal, and asserts markdown, not HTML |
| Responsive | the viewport matrix in D10 |

### D10. The viewport matrix

Six named viewports, in the `console-looks` spec: phone, tablet portrait,
tablet landscape, laptop, desktop and wide. Touch viewports use Playwright's device
presets so the pointer is real.

A dedicated spec walks every page of every look at every viewport. It asserts no
sideways scroll, reachable controls and 44 pixel touch targets, and takes a
screenshot.

It runs in the mocked browser tier (`e2e/`), which needs no cluster. The full
`e2e-live` suite runs once per look at the desktop viewport.

### D11. The install default travels as `metricsUrl` does

`console.defaultLook` goes into the console channel's opaque config, with an
entry in `configSchema`. The service reads it and returns it as
`Session.defaultLook`. The chart fails the render on a value that is neither
`workspace` nor `classic`.

### D12. Order of work

1. Introduce the seam with `classic` only. Invisible to a viewer.
2. Extract models, moving `classic` onto each.
3. Build the workspace shell and the conversations page.
4. Restyle the other pages.
5. Add the feedback behaviour.
6. Add the viewport tier and the per-look end-to-end run.
7. Carry the install default through the chart and the service.
8. Flip the build default to `workspace` last.

Until step eight the registry default is `classic`, so every earlier commit is
invisible to a viewer.

## The prototype

**Where:** `mockup/console.html` in this change. It is a single file with its
data inlined. Open it in a browser.

**The split:**

- The prototype is the reference for **composition**. Geometry, spacing, wording,
  timings and the look of each state are read from it.
- The specs are the reference for **behaviour**. They carry none of those
  numbers.

**Where the prototype is deliberately not the target:**

| The prototype | The implementation |
|---|---|
| One captured install's data, scrubbed of identifiers | Live data from the console's API |
| A Simulate menu, a fake clock and scripted events | The real stream and the real manager |
| Disconnect, fail-next and reset controls | None. They exist to demonstrate states |
| A hand-written DOM patcher | React and the model layer |
| A union-of-events graph on the Topology page with inert dropdowns | The real topology graph with its real layout and filters |
| Display and History panels that hold one line | The real panels |
| Messages have no copy control | A copy control per message, as the specs require |
| Layout kept in memory | Storage under keys of the workspace look |
| No authentication and no session | The console's real authentication |
| Overview and Queues as a snapshot | Live data |
| Colours from one dark and one light token set | The shared tokens plus `--wk-*` |

## Risks / Trade-offs

- **The end-to-end specs assert today's DOM** → the model supplies stable test
  identifiers, the specs move to them, and each runs per look.
- **Two looks double the end-to-end time** → the full suite runs once per look at
  one viewport, and the viewport matrix runs in the mocked tier.
- **The classic look rots unseen** → it is in the CI matrix and a failing classic
  run blocks a merge.
- **Moving the classic code regresses it** → step one changes imports only and
  the existing tests must stay green.
- **A look drifts from the others** → the per-look conformance run names the
  missing action.
- **The default changes for every install on upgrade** → a changelog entry, the
  `console.defaultLook` value, and the header switch.
- **The bundle grows** → looks load lazily.
- **`async-command-bus` is not finished** → the model degrades to sending then
  done.
- **The mockup carried one install's names** → the committed copy is scrubbed
  and `publication-guard.py` passes on it.

## Migration Plan

1. The upgrade makes `workspace` the default for viewers who have chosen nothing.
2. An operator who wants the old look for everyone sets
   `console.defaultLook: classic`.
3. A viewer who prefers it switches in the header, and the choice persists.
4. Rollback is the same value, or the previous chart.

## Open Questions

- The header's sign-out control exists in `classic` and does nothing under a
  shared token. The workspace look omits it. Whether `classic` should stop
  showing it is a separate fix.
- `workspace` is a working name for the new look.
