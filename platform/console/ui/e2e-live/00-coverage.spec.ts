import { test } from '@playwright/test'

// Issue #250's 18-item bug table, indexed against this suite.
//
// 15 of the 18 items each have a real, passing, UI-driven test elsewhere in
// this directory — every other spec file here is named `<item#>-*.spec.ts`
// for exactly that reason, so the report's own file list reads in issue
// order instead of alphabetically by topic. This file is `00-` so it sorts
// first, and it exists only for the three items that have NO such test —
// without it they are silently absent from the report, indistinguishable
// from forgotten. Quoted text below is verbatim from issue #250.
//
// #1, #7, #13  -> 01-inbox-attribution.spec.ts
// #2, #9       -> 02-inbox-tree.spec.ts
// #4, #5       -> 04-new-conversation.spec.ts
// #6, #14, #15 -> 06-coordinator-transcript.spec.ts
// #8           -> 08-closed-conversations.spec.ts
// #10          -> 10-selection-bar-contrast.spec.ts
// #16          -> 16-runs-view.spec.ts
// #17          -> 17-unread.spec.ts
// #18          -> 18-coordinator-attribution.spec.ts

test.skip(
  '#3 "Bulk Close / Delete looked removed" — CONFIRMED NOT A BUG. ' +
    'Issue #250\'s own proposed solution was to confirm intent, not fix a defect: ' +
    '"Not broken — confirm whether hiding bulk actions behind an explicit step is ' +
    'still wanted, or make it more discoverable." The status update on #250 records: ' +
    '"#3 and #11 confirmed as non-issues, no change made."',
  () => {},
)

test.skip(
  '#11 "Coordinator sidebar scope has no real filtering or pagination" — CONFIRMED NOT A BUG. ' +
    'Same status line as #3: "#3 and #11 confirmed as non-issues, no change made."',
  () => {},
)

test.skip(
  '#12 "No e2e coverage for the chat-shaped-conversations feature at all" — ' +
    'THIS SUITE, PLUS platform/manager/test/e2e, IS THE FIX, not one assertion. ' +
    'Issue #250\'s proposed solution: "Cover the WHOLE feature with end-to-end tests, ' +
    'not just these regressions, run against a disposable local cluster... BOTH ' +
    'conversation kinds need their own coverage" — which is why every other spec ' +
    'file in this directory exists.',
  () => {},
)
