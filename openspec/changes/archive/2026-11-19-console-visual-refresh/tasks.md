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

Sections 4.3 to 4.5, 5 and 6 and tasks 7.4, 8.3 to 8.5, 9.1, 9.2 and 10.1 to 10.10 moved to the follow-up change `console-visual-refresh-tasks`, tracked in #510.

## 7. The install default

Role: `backend-developer` for the service, `deployment-engineer` for the chart.

- [x] 7.1 Return `defaultLook` in `Session` and read it from the channel config, and verify a Go test that an unknown value is dropped
- [x] 7.2 Add `console.defaultLook` to the chart with values schema and `configSchema` entry, and verify a render test for the default and for a value that is neither look failing with both named
- [x] 7.3 Wire it into the browser's resolution order, and verify a unit test that a viewer's stored choice wins

## 8. Unit tests

Role: `testing-specialist`.

- [x] 8.1 Run `npm test` and `npm run typecheck` in `platform/console/ui` and verify both are green
- [x] 8.2 Run `go build ./... && go vet ./... && go test ./...` in `platform/console` and verify green

## 9. System tests

Role: `testing-specialist`.

- [x] 9.1 The system-tier tasks moved to the follow-up change `console-visual-refresh-tasks`, tracked in #510

## 10. Documentation

Role: `frontend-developer`.

- [x] 10.1 The reference-docs and adopter-site tasks moved to the follow-up change `console-visual-refresh-tasks`, tracked in #510
