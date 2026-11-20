## 1. Server counts — backend-developer

- [x] 1.1 Drop the `closedVisible` argument from `accumulateScopeCounts` in `platform/console/convapi.go` so every scope but `closed` counts only `phase≠Closed` unread roots, and verify `go test ./...` in `platform/console` covers each scope row of the design table
- [x] 1.2 Count `closed` as unread roots with `phase=Closed`, and verify a Go test where two of six closed roots are unread yields 2
- [x] 1.3 Add `scopes.open` and keep `unreadTotal` equal to it, delete `rootTotal`, and verify a Go test that a member and a closed root are in neither
- [x] 1.4 Stop reading `showClosed` in `parseShowClosed` and remove the function, and verify a Go test that a request carrying `showClosed=true` returns the same counts as one without it
- [x] 1.5 Make `closeOne` and `deleteOne` in `platform/console/convapi.go` skip a name whose conversation has `causedBy.parent`, with a reason naming that parent and no action taken, and verify Go tests for a close and a delete naming a member
- [x] 1.6 Keep the count-only form (`count=1`) returning the navigation count, the per-scope counts and the total with no rows, and verify a Go test of the response shape after `rootTotal` is gone
- [x] 1.7 Remove `rootTotal` from `platform/console/ui/src/api/types.ts` and `useInboxCounts`'s `showClosed` key in `hooks.ts`, and verify `npm run typecheck` is clean

## 2. Open scope and one closed rule, shared model — frontend-developer

- [x] 2.1 In `ui/src/model/conversationList.ts` replace `{ kind: 'all' }` with `{ kind: 'open' }`, drop the `showClosed` argument of `visibleItems`, `useConversationList` and `useInboxScopes`, and verify `conversationList.test.ts` shows a closed conversation listed only under Closed
- [x] 2.2 In `useInboxScopes` label the first scope Open with `scopes.open`, label the last Closed with `scopes.closed`, and verify a test of the order Open, Working, Mine, Errored, Pipelines and Coordinators, Closed
- [x] 2.3 Delete `showClosed` and its checkbox from the classic look (`looks/classic/pages/chat/layout.ts`, `ChatView.tsx`, `Inbox.tsx`) and make `open` its default scope, and verify its `ChatView` and `Inbox` tests
- [x] 2.4 Delete `showClosed` and its checkbox from the workspace look (`looks/workspace/layout.ts`, `Conversations.tsx`, `NavPanel.tsx`) and make `open` its default in `useWorkspaceScope`, and verify its `Conversations` and `Shell` tests, including the empty-list hint that mentions Show closed
- [x] 2.5 Verify a test in each look that a stored `showClosed` value is ignored and `readLayout` still never throws
- [x] 2.6 Show the Open and Closed badges in each look's expanded and collapsed inbox, and verify a badge test for both

## 3. Selection over roots — frontend-developer

- [x] 3.1 Make `selectableNames` and `selectionState` in `ui/src/model` pick roots only and skip a root already closing or being deleted, hide the member row's selection control in both looks, and verify tests for a grouped and a flattened list
- [x] 3.2 Remove the skip branch from `partitionSelection` in `ui/src/model/tree.ts` and the `skippedResults` merge from both looks' `SelectionBar`, and verify `tree.test.ts` and both `SelectionBar` tests have no member-skip case left
- [x] 3.3 Make "N selected" and the `Close (n)` and `Delete (n)` suffixes count selected roots, and verify a test of two roots with five members reading 2
- [x] 3.4 Make the close and delete dialog titles count roots only and keep `memberCount` on its own line, in `CloseConversations`, `DeleteConversations` and the workspace `BatchDialogs`, and verify the dialog tests
- [x] 3.5 Add the working-member count to the member line, and verify a test of three members with one working
- [x] 3.6 Make the result banners report per-root outcomes only, and verify the close and delete result tests

## 4. One word for the state — frontend-developer

- [x] 4.1 Rename Archive to Closed in the scope title (`SCOPE_LABEL` in the workspace `Conversations.tsx`) and the classic and workspace thread banners, and verify the `Thread` and `ThreadPane` tests
- [x] 4.2 Change the close dialog text in both looks and the `/close` chip text in `QuickChips.tsx` and the workspace `Composer.tsx`, and verify their tests
- [x] 4.3 Add the retired inbox wording ("Show closed", the Archive row, the All scope) to `.github/retired-vocabulary.json` with Open and Closed as replacements, and verify `retired-vocabulary-guard.py` is clean

## 5. Visual check — frontend-developer

- [x] 5.1 Run the dev server from this working copy's `platform/console/ui`, screenshot Open, Closed, a Pipeline scope, the selection bar and both dialogs in BOTH looks, and read the PNGs per `visual-check.md`. **Not run against a live cluster** — this remote session has none (`visual-check.md`'s dev-server procedure needs a port-forwarded install, and `remote-session.md` names the visual check workstation-only). Verified instead with the built bundle driven by Playwright against the repo's own mocked harnesses (the same mechanism `npm run screenshots` and `e2e/` use): Open, Closed, a Pipeline scope, the selection bar (2 selected / Close…(2) / Delete (2), roots only) and both close dialogs (root-only title, member line with working-member count on its own line), read in both looks and both themes. Found and fixed one stale fixture left over from section 1: `e2e/support/mock.ts`'s canned `/api/conversations` response still carried `rootTotal: 5` (removed from the real wire type) and no `scopes.open` key.

## 6. Unit tests

- [x] 6.1 Run `npm test` and `npm run typecheck` in `platform/console/ui` and verify both are green
- [x] 6.2 Run `go build ./... && go vet ./... && go test ./...` in `platform/console` and verify green
- [x] 6.3 Verify the coverage flags `build-test.md` names leave no new behaviour uncovered, including the ignored `showClosed` in both looks

## 7. System tests

- [x] 7.1 State why the cluster tier does not apply, since nothing here is decided by a kubelet, RBAC or an informer, and the console's browser and Go tiers in sections 1 to 4 carry the proof. Confirmed: `go test` (90.1% coverage), `npm test` (951 unit tests) and `npm run test:e2e` (the mocked `playwright test` browser tier, 33/33) are all green, covering both browser tiers `change-tests.md` names for a console change. No manager, CRD, chart, RBAC or informer behaviour changes (design.md's own Non-Goals), so no system-tier lane applies.

## 8. Documentation

Reference docs:

- [x] 8.1 Update the scopes table in `docs/console.md` for Open and Closed, the unread-roots rule, and remove the "Show closed" paragraphs and the layout-preference mention
- [x] 8.2 Update `docs/console.md` for root-only selection and the member line in the dialogs
- [x] 8.3 Update `docs/console-guide.md` where it names All, Archive or the checkbox. Only the conversations-light.png alt text named any of them (a closed row no longer visible under Open), and it is fixed. "Every scope carries its own sum" names none of the three and was left alone
- [x] 8.4 Add a `docs/CHANGELOG.md` entry that `showClosed` and `rootTotal` leave the console's conversations API and the Closed badge is now unread-only
- [x] 8.5 Add the counter table from `design.md` to `docs/console.md`, or link it from there, and verify it matches the code. Added as its own `## Counter reference` section, dropping the table's "Change" column (states what the proposal changed, not what IS) and spot-verified against `accumulateScopeCounts`, `selectionState`, `SelectionBar.tsx`, `CloseConversations.tsx` and `DeleteConversations.tsx`

Adopter site:

- [x] 8.6 Update the landing page, `docs/introduction.md` and `docs/getting-started.md` where they show the inbox. None of the three names All/Archive/the checkbox or embeds a conversations screenshot — `index.md`'s only console asset is the demo recording's poster, regenerated by 8.7 instead of hand-edited
- [ ] 8.7 Re-run `npm run screenshots` and `npm run demo` in `platform/console/ui` and verify the assets changed. `screenshots` succeeded, all 20 PNGs changed, spot-checked against both themes. `demo` built the light frames and updated `console-demo-poster-light.png`, then failed encoding both themes' MP4 and the dark poster: this sandbox's `dockerd` cannot start (`ulimit: Operation not permitted`), and the encode step shells out to `docker run mwader/static-ffmpeg`. `console-demo-poster-dark.png`, `console-demo-light.mp4` and `console-demo-dark.mp4` are therefore UNCHANGED and still show the retired "All conversations" / "Show closed" / "Archive" wording — confirmed by reading the stale dark poster. Needs a re-run with Docker available before this is fully shipped
- [x] 8.8 Run `publication-guard.py`, `retired-vocabulary-guard.py` and `docs-generate.py --check` and verify a clean verdict. All three clean
