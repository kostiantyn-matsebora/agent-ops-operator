## 1. Shell

- [ ] 1.1 In `platform/console/ui/src/App.tsx`, switch `Page` to `isManagedSidebar`, add a `PageToggleButton` inside `MastheadToggle`, and delete the `navOpen` state and `onPageResize` handler; verify with `npm run build` in the worktree's `platform/console/ui` and a Playwright shot at 390×844 (dev server per `visual-check.md`, PNG to the scratchpad) showing the sidebar hidden and the toggle present.

## 2. Tables

- [ ] 2.1 Add `gridBreakPoint="grid-md"` and a `dataLabel` on every `Td` (matching its `Th` text) to the three tables in `Overview.tsx`, hiding Image and Source per design.md via `visibility`; verify `npm test` passes and a 390px shot shows labelled cards.
- [ ] 2.2 The same for `Config.tsx`'s kind list, conditions and findings tables (hide Labels, Age, Since); verify as 2.1.
- [ ] 2.3 The same for `Queues.tsx`'s three tables and `Vocabulary.tsx`'s two; verify as 2.1.
- [ ] 2.4 `Conversations.tsx`: add `gridBreakPoint`, hide Runs, Queued and Console below `lg`, give the select and reopen cells explicit narrow handling, and collapse the toolbar's filters behind `collapseListedFiltersBreakpoint="md"` with the search box kept visible; verify `Conversations.test.tsx` still passes and a 390px shot shows one card per conversation with labels and the search box.
- [ ] 2.5 `Conversation.tsx`: add `gridBreakPoint` to the run timeline table keeping Timeline visible; verify at 390px that the transcript, the composer with its command menu open, and the timeline each fit without sideways scroll.

## 3. Verification

- [ ] 3.1 Add `platform/console/ui/e2e/phone.spec.ts` at a 390×844 viewport asserting, for every route, `scrollWidth <= innerWidth`, the sidebar hidden with the toggle present, and the conversations list rendering cards with data labels; verify it passes with `npx playwright test e2e/phone.spec.ts` in the worktree and that the existing e2e specs still pass.
- [ ] 3.2 Build the console image from the worktree and deploy it with `helmfile sync --state-values-set chartPath=<worktree>/chart`; verify against the live install with a port-forwarded 390×844 shot of a real conversation.

## 4. Documentation

### 4.1 Reference docs

- [ ] 4.1.1 Confirm `docs/console.md` and `docs/CHANGELOG.md` need no entry (no endpoint, value or breaking change) and record that in the pull request description.

### 4.2 Adopter site

- [ ] 4.2.1 `docs/console-guide.md`: add to the tour that the console renders on a phone — the navigation opens from the masthead, lists become cards — and that the conversation page is the one to keep open there; verify the docs lint in `docs/CLAUDE.md` passes.
- [ ] 4.2.2 Re-run BOTH `npm run screenshots` and `npm run demo` in `platform/console/ui` from the worktree and commit the regenerated site assets; verify `git status` shows the console PNGs and the landing recording updated and the masthead toggle visible in them.
