## Why

The console is a phone's-worth of value — someone paged by a chat channel wants
to read the conversation and reply from wherever they are — and it does not
render on one. Every table overflows sideways, and the sidebar is pinned open
at every width because the shell forces it open on resize, so the transcript
that already fits a narrow column is reached through a shell that does not.
The gap is missing responsive props on components that already have the
mechanism, not a missing product; a separate mobile app was considered and
rejected (see design.md).

## What Changes

- The page shell adopts the managed sidebar: collapsed below the tablet
  breakpoint, opened from a toggle in the masthead, and the resize handler
  that forced it open is removed.
- Every data table stacks into labelled cards below the tablet breakpoint,
  and the columns that carry least on a narrow screen are hidden there rather
  than stacked.
- The Conversations toolbar collapses its filters behind a toggle below the
  tablet breakpoint, so the search box keeps the width.
- The conversation page is verified at a phone viewport: the transcript,
  the composer with its upward command menu, and the run timeline.
- A browser test asserts that no console page scrolls horizontally at a
  phone viewport, so the property is kept rather than restored.

No API, CRD, chart value or contract changes.

## Capabilities

### New Capabilities
- `console-responsive`: the console renders on a narrow viewport — a
  collapsible navigation, tables that stack, and no page that scrolls
  sideways.

### Modified Capabilities

(none — `console-application` states what each page reports, not its layout)

## Impact

- `platform/console/ui/src/App.tsx` — shell and masthead.
- `platform/console/ui/src/pages/{Overview,Config,Queues,Vocabulary,Conversations,Conversation}.tsx`
  — table props, column visibility, toolbar collapse.
- `platform/console/ui/e2e/` — a new phone-viewport spec.
- No Go change; the console image ships a rebuilt bundle.

Documents this makes untrue, both halves:

- Reference docs: none — `docs/console.md` states endpoints, RBAC and values,
  none of which move; `docs/CHANGELOG.md` records breaking changes only.
- Adopter site: `docs/console-guide.md` — the tour describes the shell and
  does not say the console works on a phone; the site's console screenshots
  and the landing recording are build output of a UI this change alters
  (the masthead gains a toggle) and are regenerated. The landing page,
  Introduction, Getting started and Installation make no claim about the
  console's layout and stay as they are.
