## Context

See proposal.md — Why. The console is one PatternFly 6 SPA, embedded in the
adapter image via `go:embed`, served under a proxy, and rendered by two
Playwright harnesses (`screenshots/`, `demo/`) whose output is the site's.
The relevant state today:

- `App.tsx` holds `navOpen` state and `onPageResize={() => setNavOpen(true)}`;
  the masthead renders no toggle. PatternFly's own collapse-under-`md`
  behaviour exists and is defeated by that handler.
- Eleven `@patternfly/react-table` tables across `Overview`, `Config`,
  `Queues`, `Vocabulary`, `Conversations` and `Conversation` (the run
  timeline). Only `Conversations` and the timeline pass `dataLabel`; none
  pass `gridBreakPoint`, which is what switches to the stacked layout.
- The transcript is already a `2em 1fr` grid with `minWidth: 0` and
  `overflowX: hidden`, so it fits. The composer's command menu is
  absolutely positioned above the field.
- `index.html` already ships a `width=device-width` viewport meta.

No prototype was made: the composition is PatternFly's stacked-table and
managed-sidebar layouts as shipped, so there is no geometry of ours to record.

## Goals / Non-Goals

**Goals:**
- Every page usable on a 390px viewport; the conversation pages good on one.
- No new dependency, no second bundle, no per-device code path.

**Non-Goals:**
- A phone-first chat client (thread list as home, composer pinned to a
  bottom bar, notifications). That is a product change on top of this one.
- Touch gestures, offline, PWA manifest.
- Redesigning any table's columns for desktop.

## Decisions

**Responsive rendering, not a separate app.** A second app is a second
component under `platform/`, a second published image, a second copy of
`blocks.ts` (already one of two parsers that must change together), and a
second screenshot harness — for what is, in the code, missing props. The one
page a phone user wants already fits.

**PatternFly's managed sidebar.** `Page isManagedSidebar` plus a
`PageToggleButton` in `MastheadToggle`; the `navOpen` state and the resize
handler go. Alternative — keep our own state and add a media query — rebuilds
what the component does and keeps the bug's shape.

**One breakpoint for every table: `gridBreakPoint="grid-md"`.** PatternFly's
`md` is 768px, which puts phones in card mode and tablets in table mode.
Alternative `grid-lg` would stack on tablets, where the tables fit. Every
`Td` gets a `dataLabel` matching its `Th` text — the stacked layout prints
the label, and a cell without one prints nothing.

**Hide, do not stack, the low-value columns.** Via the `visibility` prop on
the paired `Th`/`Td`, hidden below `lg`:

| Table | Hidden on narrow |
|---|---|
| conversations | Runs, Queued, Console |
| workloads | Image |
| adapters | Image |
| problems | Source |
| config kind list | Labels, Age |
| conditions | Since |

Everything else stacks. A card of nine labelled rows per conversation is
what the alternative — stack everything — produces, and it buries the title.

**Conversations toolbar collapses its filters.** PatternFly `Toolbar
collapseListedFiltersBreakpoint="md"` with the search box outside the
filter group, so it keeps the width and the three selects fold behind the
toolbar's own toggle.

**The property is asserted by a browser test, not a screenshot.** A new
`e2e/phone.spec.ts` at 390×844 visits every route against the fixture and
asserts `document.scrollingElement.scrollWidth <= innerWidth`, plus the
sidebar-hidden and conversations-card checks. Screenshots verify the change
once; the assertion keeps it.

**The site's screenshots stay desktop.** The harnesses' viewport is
unchanged; they are re-run because the masthead now carries a toggle. A
phone screenshot for the guide is not added — the guide gets a sentence.

## Risks / Trade-offs

- [The run timeline's percentage bars are laid out in a table cell; in card
  mode the Timeline cell is full width, which is fine, but the bars' track
  must not be hidden] → keep Timeline stacked, hide nothing there; check it
  in the phone spec.
- [PatternFly's stacked table drops the header row, so a `Th` with
  `screenReaderText` and no `dataLabel` on its `Td` prints an unlabelled
  value] → the reopen and select cells get explicit handling; the spec's
  conversations scenario checks the labels.
- [The topology graph is a canvas the viewport cannot stack] → out of
  scope beyond not overflowing; it pans and zooms already.
- [`isManagedSidebar` changes the initial sidebar state the screenshot
  harness sees] → the harness viewport is above `md`, so the sidebar is
  open as before; confirmed by re-running it.
