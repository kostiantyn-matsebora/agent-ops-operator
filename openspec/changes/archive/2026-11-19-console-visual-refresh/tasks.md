## 1. The seam, with classic only

Role: `frontend-developer`. Nothing here is visible to a viewer.

- [x] 1.1 Add `looks/registry.ts` with `classic` as the only entry, and verify a unit test lists it
- [x] 1.2 Move today's `pages/` and `components/` into `looks/classic/` with only import paths edited, and verify the existing `npm test` and `npm run typecheck` stay green
- [x] 1.3 Add the `useLookStore` beside the colour-scheme store with key `agentops-console-look`, and verify resolution order with a unit test (query, stored, install default, build default)
- [x] 1.4 Set `data-look` on the root and lazy-load the active look, and verify a test that only the active look's chunk loads
- [x] 1.5 Add the look switcher to the shared header and verify it lists every registered look and marks the active one
- [x] 1.6 Add the import-boundary test (model imports no component library or look, a look imports no other look) and verify it fails on a planted violation

## 2. The model layer

Role: `frontend-developer`. Move `classic` onto each model as it lands.

- [x] 2.1 Extract the conversation list model (scopes, counts, ordering, unread) and verify unit tests pin its output against the current fixtures
- [x] 2.2 Extract the thread model (messages with their source markdown, signals, runs, members, start time) and verify the `classic` thread renders unchanged
- [x] 2.3 Extract the working-run model as `{ elapsed, latest, count }` that attaches tools and events to the reply on arrival, and verify a test for a viewer-sent run and a signal-started run
- [x] 2.4 Add `useCommands` with the states sending, accepted, done and failed, the in-flight count and the connection state, and verify unit tests for each transition and for retry on a safe verb only
- [x] 2.5 Make `accepted` optional so a synchronous verb goes from sending to done, and verify with a test
- [x] 2.6 Publish stable test identifiers from the model contract and verify the `e2e-live` specs use them in place of classes

## 3. The workspace look: shell and conversations

Role: `frontend-developer`. PORT from `mockup/console.html`: composition, spacing, wording and timings come from it. Behaviour comes from the specs.

- [x] 3.1 Build the workspace shell (header, navigation panel, collapse, drawer) with `--wk-*` tokens, and verify a screenshot at laptop width against the prototype
- [x] 3.2 Make the shell fill the viewport with no width cap, and verify a 2560 by 1300 spec asserts the panes reach both edges and the page does not scroll
- [x] 3.3 Build the list row with a fixed height, a reserved lifecycle line and the right-hand time, badge and status column, and verify a test that a status change moves no row
- [x] 3.4 Build the thread with the date label under the tabs, day dividers only on a day change, and signal cards with severity as an icon, and verify a unit test per contract value
- [x] 3.5 Build the folded run detail with the always-visible `Invoked` row, and verify a spec that a finished reply shows only the reply, `Invoked` and the collapsed `Run details`
- [x] 3.6 Add the quiet inline code style and the block form for long commands, and verify a screenshot at both colour schemes
- [x] 3.7 Add a copy control to every message, copying markdown, with in-place confirmation and a selected-text fallback, and verify a spec reads the clipboard for a reply, a person's message and a signal
- [x] 3.8 Persist pane sizes and collapsed states under keys of this look, and verify they survive a reload
- [x] 3.9 Keep every `--wk-*` token in `looks/workspace/workspace.css` and out of `theme/theme.css`, and verify by grep that `theme.css` holds none and the site CSS needs none
- [x] 3.10 If the work changes any shared `--ao-*` value in `theme/theme.css`, copy it into the token block at the head of `docs/assets/css/agentops.css` as `palette-and-mark.md` requires, and verify the hex grep from that rule finds hits only in the token blocks

- [x] 3.11 Give the workspace conversations page the actions `classic` has and the first cut lacks: Graph and Sequence tabs, select and bulk actions, reopen and delete, the escalation bar, the jump-to-new pill, slash typeahead and quick chips, and verify a per-look conformance test that names any action one look exposes and the other omits

## 4. The workspace look: the other pages

Role: `frontend-developer`. Restyle only. Each page keeps the sections, order and fields it has in `classic`.

- [x] 4.1 Overview, and verify a test that both looks render the same cards in the same order
- [x] 4.2 Queues, with the same check
- [x] 4.3 Configuration with its kind index, kind tables, object pages and the Overview and YAML tabs, with the same check — moved to #510
- [x] 4.4 Topology with its real graph, filters and panels, and verify the `e2e/topology.spec.ts` assertions pass in both looks — moved to #510
- [x] 4.5 Show a sign-out control only where authentication offers one, and verify a test under a shared token — moved to #510

## 5. Feedback and motion

Role: `frontend-developer`. Task 5.5 is checked against WCAG 2.3.3, animation from interactions.

- [x] 5.1 Show command states on the row and in the thread, and verify a spec that a reply goes from sending to delivered on its event — moved to #510
- [x] 5.2 Show the in-flight counter and the connection state in the header, and verify a spec that a dropped stream pauses and a return resumes — moved to #510
- [x] 5.3 Make arrivals appear in place with no banner, toast or modal, and verify a spec on a signal-opened conversation — moved to #510
- [x] 5.4 Show the working run as one fixed-size card that folds into `Run details` on the reply, and verify no per-event line remains — moved to #510
- [x] 5.5 Honour reduced motion, and verify a spec with `prefers-reduced-motion` that no animation runs and state is still conveyed — moved to #510
- [x] 5.6 Add the layout-shift check, observing `layout-shift` entries and polling header, tab bar and row rects through a simulated run, and verify it reports nothing beyond the one insertion — moved to #510

## 6. Viewports

Role: `testing-specialist`.

- [x] 6.1 Define the six named viewports once (phone, tablet portrait, tablet landscape, laptop, desktop, wide) with Playwright device presets for touch, and verify the list matches `console-looks` — moved to #510
- [x] 6.2 Add a mocked-tier spec that loads every page of every look at every viewport and asserts no sideways scroll, reachable controls and 44 pixel touch targets, and verify it fails on a planted overflow — moved to #510
- [x] 6.3 Parametrise the `e2e-live` suite to run once per look at the desktop viewport, pinning each with `?look=`, and verify the nightly job runs both — moved to #510
- [x] 6.4 Check the tablet layouts in both orientations by screenshot and read the images, per `visual-check.md` — moved to #510

## 7. The install default

Role: `backend-developer` for the service, `deployment-engineer` for the chart.

- [x] 7.1 Return `defaultLook` in `Session` and read it from the channel config, and verify a Go test that an unknown value is dropped
- [x] 7.2 Add `console.defaultLook` to the chart with values schema and `configSchema` entry, and verify a render test for the default and for a value that is neither look failing with both named
- [x] 7.3 Wire it into the browser's resolution order, and verify a unit test that a viewer's stored choice wins
- [x] 7.4 Flip the build default to `workspace` and verify the first-visit test — moved to #510

## 8. Unit tests

Role: `testing-specialist`.

- [x] 8.1 Run `npm test` and `npm run typecheck` in `platform/console/ui` and verify both are green
- [x] 8.2 Run `go build ./... && go vet ./... && go test ./...` in `platform/console` and verify green
- [x] 8.3 Run the chart render tests and verify green — moved to #510
- [x] 8.4 Verify coverage with the flags `build-test.md` names has no uncovered new behaviour — moved to #510
- [x] 8.5 Run the mocked browser tier (`npm run test:e2e`, the `console-e2e` gate) for every look, including the viewport matrix from 6.2, and verify green — moved to #510

## 9. System tests

Role: `testing-specialist`.

- [x] 9.1 State why the manager's system pack gains no lane, since nothing here is decided by a kubelet, RBAC or an informer — moved to #510
- [x] 9.2 Run the `e2e-live` suite once per look against a real install (k3d or a local cluster, per `docs/testing.md`), and verify both runs pass with the specs reading only stable test identifiers — moved to #510

## 10. Documentation

Role: `frontend-developer`.

Reference docs:

- [x] 10.1 Update `docs/console.md` for both looks and the switcher — moved to #510
- [x] 10.2 Update `docs/console-guide.md` so the tour names the look it shows — moved to #510
- [x] 10.3 Update `docs/testing.md` for the per-look and per-viewport browser tiers — moved to #510
- [x] 10.4 Add `console.defaultLook` to `docs/configuration.md` and re-run `docs-generate.py` — moved to #510
- [x] 10.5 Add a `docs/CHANGELOG.md` entry that the default look changes and how to keep `classic` — moved to #510
- [x] 10.6 Update `.claude/rules/palette-and-mark.md` to say which tokens are shared with the site and which belong to a look — moved to #510

Adopter site:

- [x] 10.7 Update the landing page, `docs/introduction.md` and `docs/getting-started.md` where they show the console — moved to #510
- [x] 10.8 Update `docs/installation.md` where it shows the console's values — moved to #510
- [x] 10.9 Re-run `npm run screenshots` and `npm run demo` for both looks and verify the assets changed — moved to #510
- [x] 10.10 Run `publication-guard.py`, `retired-vocabulary-guard.py` and `docs-generate.py --check` and verify a clean verdict — moved to #510
