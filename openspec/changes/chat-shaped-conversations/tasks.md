Every build and test below runs INSIDE the worktree (`docker exec -w "$PWD"`
from `../agent-ops-worktrees/chat-shaped-conversations` for Go, `npm` under
`platform/console/ui` of that same tree for the UI). PORT the composition from
`prototype/` — `C-rail.html`, `C-collapsed.html`, `D-incident.html`,
`States.html` — and read behaviour from `specs/`. Never re-derive either.

## 1. The count and the row fields (console, Go)

- [x] 1.1 `platform/console/transcript.go`: a `countedKinds` set (`signal`, `agent`, `relay`) and `countUnread(messages, watermark)` returning the count and the newest counted message. Verify: table test over every kind, own messages and acks excluded.
- [x] 1.2 `platform/console/conversations.go`: `ConversationSummary` gains `unreadCount`, `lastMessage{kind,sender,text}` and `presence` (inflight), computed from `status.runs[]` plus the live buffer for the console thread and the reader. `Unread` becomes `unreadCount > 0`. A conversation with no console thread carries neither. Verify: `go test ./...` with a fixture of ten runs and a buffered ack.
- [x] 1.3 `platform/console/convapi.go`: the unread filter and `unreadTotal` use the count rule, `unreadTotal` becomes the sum, the count-only form adds per-scope counts (filters, each pipeline and coordinator), and the `mine` filter exists. An `incidents` filter was built here too, then removed outright once the Incidents scope itself was dropped from the inbox (`console-chat-layout`) — `convapi_scope_test.go` now asserts it is never computed. Verify: handler tests for the sum, a scope count, and the `mine` filter.
- [x] 1.4 `platform/console/convapi.go` and `api/apply.ts`: the applied `conversationRow` on a delta carries the new fields, so a live row moves without a refetch. Verify: `apply.test.ts` asserts a delta updates `unreadCount`.
- [x] 1.5 The read report on open sends `max(newest counted message time, lastActivity)` (design D-D). Verify: a test where dispatch stamped `lastActivity` after the answer still reports once.

## 2. Mark unread, and the close/delete cascade (manager and console)

- [x] 2.1 `platform/manager/internal/httpapi/channels.go` and the read-report type: an entry may carry `rewind: true`. The manager sets the named reader's entry to `readAt`, clamps to now, leaves the channel-wide mark, and refuses a rewind with no reader. `ThreadBinding.ReadAt` / `ReaderMark.ReadAt` widened from `*metav1.Time` (API-server SECOND granularity, silently dropping the sub-second component the rewind needs) to an opaque RFC3339Nano `*string` — found as a real production defect by the e2e pack's `TestConsoleMarkReadThenUnreadRewind` (task 8.3). Verify: envtest cases for rewind, no-reader refusal, the channel-wide mark untouched, and the sub-second round-trip (`TestChannelReadPreservesSubSecondPrecision`).
- [x] 2.2 `platform/console/adapter.go` and `convapi.go`: `POST /api/conversations/unread` over names, bounded at 50, attributed, absent without a reader salt. Sends the newest counted message's time minus one nanosecond. Verify: handler tests for the bound, the refusal without a reader, and the resulting count of one.
- [x] 2.3 `platform/manager/internal/chat/router.go`: `closeConversation` (reached by `CloseConversation` and `AutoCloseConversation`) cascades to every live direct member after closing itself, calling the same `cascadeCloseMembers` helper `coordinate.go`'s `CloseCoordinated` already uses, with no reason required or threaded through for a non-coordinator-issued close. Verify: a test closing a root via `/close` finds its members `Closed` too, recursively two levels deep, and a plain conversation with no members closes exactly as before.
- [x] 2.4 `platform/manager/internal/httpapi/server.go`: `handleConversationDelete` lists conversations labelled `agentops.dev/caused-by=<name>`, confirms each one's `causedBy.parent` against the fact, and deletes every one found before deleting the named conversation, recursively, tolerating `NotFound`. Verify: envtest deleting a root with two members, one itself a root with its own member, deletes all four. Deleting a plain closed conversation with no descendants is unchanged.

## 3. The view (UI layout)

- [x] 3.1 `pages/chat/Splitter.tsx`: drag, double-click reset, keyboard steps, minimum widths per design D-B. Verify: `Splitter.test.tsx` covers drag clamp, reset and keyboard.
- [x] 3.2 `pages/chat/layout.ts`: the `agentops.console.layout` key, guarded read and write, defaults from `prototype/C-rail.html`. Verify: tests with storage present, throwing and absent.
- [x] 3.3 `pages/chat/ChatView.tsx` and `App.tsx`: both conversation routes render the view, the PatternFly sidebar collapses to icons on it, and the narrow-window mode shows one column. Verify: `ChatView.test.tsx` renders both routes and the narrow mode.
- [x] 3.4 `pages/chat/Inbox.tsx`: scopes in spec order with counts from the count-only form, the collapse control, and the collapsed strip with badges, PORTED from `C-rail.html` and `C-collapsed.html`. Verify: `Inbox.test.tsx` asserts order, counts and the collapsed badges.
- [x] 3.5 Keyboard: `↑`/`↓`, `Enter`, `⌘`-click, `Esc` per the layout spec. Verify: a test walks three rows and opens one.

## 4. The list (rows, tree, selection)

- [x] 4.1 `pages/chat/ConversationRow.tsx`: avatar with phase dot, title, time, snippet from `lastMessage`, count badge or tag, PORTED from `States.html`. Verify: `ConversationRow.test.tsx` covers every row state on that board.
- [x] 4.2 `pages/chat/tree.ts`: group by `causedBy.root`, depth from the parent chain, flatten toggle, root row extras (caret, member count, turn, deadline), the autosolved marker. Verify: `tree.test.ts` with a one-level and a two-level fixture.
- [x] 4.3 Arrival: a new row enters with the tint and the `new` marker, a toast names the pipeline, reduced motion honoured. Verify: a test asserts the marker and the toast, and no animation class under reduced motion.
- [x] 4.4 `pages/chat/SelectionBar.tsx` and the checkbox column: Select and `⌘`-click enter the mode, the bar offers Mark read, Mark unread, Close…, Delete with today's rules, rows held by a finalizer unselectable. A root's confirmation counts its members for BOTH Close and Delete, and a member reached directly by the selection reports `skipped` naming its parent rather than acting on it. Verify: the existing close and delete tests pass against the bar, plus a mark-unread test, a member-count test for both actions, and a member-in-selection skip test for both.
- [x] 4.5 `pages/chat/RowMenu.tsx`: the menu per the quick-actions spec, absent items rather than disabled ones. A member row offers only open in a new tab, copy link and open incident. Verify: `RowMenu.test.tsx` covers closed, member and working rows, asserting the member row carries none of mark unread/read, reopen, exit runtime, close or delete.

## 5. The thread pane

- [x] 5.1 `pages/chat/ThreadPane.tsx`: header with title, presence chip, bound channels, the secondary views (Runs, Graph, Sequence, YAML) replacing the transcript in place. Verify: a test switches to Runs and back.
- [x] 5.2 `pages/chat/Timeline.tsx`: messages interleaved with run events from `detail.events`, folded per run, the activity gap marker, acks rendered as the presence row. Verify: `Timeline.test.tsx` asserts order, folding and that an ack is not a bubble.
- [x] 5.3 The new-messages divider above the first uncounted message, open scrolls to it, autoscroll only at the bottom, the jump pill with its count. Verify: tests for divider placement and for no scroll while scrolled up.
- [x] 5.4 Incident timeline on a root: invocation lines, result cards with transcript links, nested cards for a coordinating member, the escalation divider, read-only before escalation with the reason, the parent chain in a member's header, PORTED from `D-incident.html`. Verify: tests over a two-level fixture.
- [x] 5.5 `pages/chat/QuickChips.tsx`: start chips from the vocabulary opening `NewConversation` with `/<name> ` prefilled, thread command chips, choice chips. Absent when writes are off. Verify: `QuickChips.test.tsx` covers all three and the read-only case.
- [x] 5.6 Delete `pages/Conversations.tsx` and `pages/Conversation.tsx` and move their surviving tests. Verify: `npm run typecheck` and `npm test` pass with no reference to either file.

## 6. Fixture and assets

- [x] 6.1 `screenshots/fixture.ts`: a working-and-read conversation, a two-level incident, an autosolved root, and a relay, so both assets show the rules. Verify: the fixture type-checks and the capture spec targets the view.
- [x] 6.2 `demo/story.ts`: the beats walk the new view (a conversation arrives, is opened in place, a reply, the answer). Verify: `npm run demo` produces frames in the worktree's `docs/assets/video/`.

## 7. Unit tests

- [x] 7.1 `cd platform/console && go test -count=1 -coverpkg=./... ./...` passes in the worktree.
- [x] 7.2 `cd platform/manager && go test -count=1 ./internal/httpapi/... ./internal/chat/...` passes in the worktree, with `KUBEBUILDER_ASSETS` set for the envtest cases in 2.1, 2.3 and 2.4.
- [x] 7.3 `cd platform/console/ui && npm run typecheck && npm test` passes in the worktree, with every test named in sections 1 to 5 present.
- [x] 7.4 `python3 .github/scripts/publication-guard.py` and `python3 .github/scripts/retired-vocabulary-guard.py` pass on the worktree, including `prototype/`.

## 8. E2E tests

- [x] 8.1 `platform/console/ui/e2e-live/`: a new Playwright tier — 21 specs plus `global-setup.ts` and `support/` — against a REAL cluster, with a real browser, console and manager, no mocks and no envtest.
  1. Built because issue #250 flagged **"No e2e coverage for the chat-shaped-conversations feature at all"** and asked for coverage of BOTH conversation kinds.
  2. The proposal's original "nothing here is decided by a cluster" claim was wrong. QA against the real view found bugs unit tests and envtest missed.
  3. `00-coverage.spec.ts`'s own header maps every one of issue #250's 18 items to the spec file that fixes it.
  4. Verify, manual only, per `e2e-live/playwright.config.ts` and `global-setup.ts`: point `E2E_LIVE_CONSOLE_URL` at a port-forwarded console on a real cluster, with `kubectl` on PATH against `E2E_LIVE_KUBE_CONTEXT` (default `k3d-agentops-e2e`).
  5. Then run `npx playwright test --config e2e-live/playwright.config.ts` from `platform/console/ui`.
  6. **Not CI-gated.** No workflow invokes it and no `package.json` script runs it. `test:e2e` is the pre-existing mocked-API smoke, a different config.
  7. **Workstation-only**, on the footing `.claude/rules/remote-session.md` gives "the local cluster". The cloud bootstrap installs no k3d or docker, so a remote session cannot satisfy `global-setup.ts`.
- [x] 8.2 `platform/manager/test/e2e/{coordinator.go,coordinator_test.go,coordinator_chat_test.go,wiring.go}`: new full-tier lanes for coordinator mode — a real pod per member conversation, a real cron-adapter-fired signal admitted on its own schedule, the deployed manager's `/coordinate/*` surface reached over the network with a derived token, and a Coordinator addressed from chat whose root binds its channels at creation and is delivered to over real channel adapters (the console's own HTTP surface and the Telegram lane against the fake Bot API) — the live substrate facts no envtest (no kubelet) and no Playwright spec (no MCP token) can settle. Verify: `go test -tags e2e -count=1 -timeout 45m -v ./test/e2e/` with `E2E_TIER=full`.
- [x] 8.3 `platform/manager/test/e2e/lifecycle_test.go`: `TestConsolePlainConversationBulkCloseAndDelete` and `TestConsoleMarkReadThenUnreadRewind`, against the same deployed console and manager as 8.2 — the console's bulk close/delete surface proven against an ORDINARY pipeline conversation (previously only exercised against a coordinator), and the mark-unread rewind proven end to end. **`TestConsoleMarkReadThenUnreadRewind` is the test that found a real production defect**, documented in its own comment: `ThreadBinding.ReadAt` / `ReaderMark.ReadAt` were `*metav1.Time`, which the Kubernetes API serializes at SECOND granularity, so the rewind's watermark silently lost its sub-second component and the message it was meant to cover counted as unread forever. Fixed by widening both fields to an opaque RFC3339Nano string (`channelread.go`, `conversation_types.go`). `internal/integration/channelread_test.go`'s `TestChannelReadPreservesSubSecondPrecision` now pins the same round-trip against the envtest API server directly, so this e2e lane is not the only place the fix is proven — it proves the console's own LIVE rendering agrees. Verify: same command as 8.2.

## 9. Documentation

### 9.1 Reference docs

- [x] 9.1.1 `docs/console.md`: the Conversations section (one view, the four columns, the splitters, the collapsed inbox, the secondary views), the Unread section (the counting rule by kind, the sum, mark unread as a reader-scoped rewind, and the "no mark as unread" paragraph removed), "What the browser keeps" (layout preferences persist, conversation state does not), the Closing and Deleting sections (closing cascades to every live descendant and deleting to every already-closed one, the selection bar and the row menu say so), the Reopening section, and the coordination tree.
- [x] 9.1.2 `docs/concepts.md`: the read-state section names the console's counting rule and the rewind form beside the thread rule. The Closing and Deletion sections state the cascade through `causedBy`, for every originator.
- [x] 9.1.3 `docs/contracts.md`: `POST /channel/read` documents `rewind`.
- [x] 9.1.4 `docs/CHANGELOG.md`: an Unreleased entry for the console image naming the new view, the counting rule, mark unread, and both persisted keys (`agentops.console.layout`, `agentops.console.skipCloseConfirm`), and a manager entry for the close/delete cascade.
- [x] 9.1.5 `.claude/rules/structure.md`: the console section names `pages/chat/` and both browser-persisted keys (`agentops.console.layout`, `agentops.console.skipCloseConfirm`).
- [x] 9.1.6 The CRD DID change here — `status.runs[].inputs[].origin` (new field) and `status.threads[].readAt` / `.readers[].readAt` (type change, same JSON field name). `python3 .github/scripts/docs-generate.py` regenerates `docs/cr-reference.md` for the new field, and `python3 .github/scripts/docs-generate.py --check` passes against the regenerated file.

### 9.2 Adopter site

- [x] 9.2.1 `docs/console-guide.md`: the Conversations and Conversation tour entries describe the one view, the counting rule and the tree, with new captions.
- [x] 9.2.2 `cd platform/console/ui && npm run screenshots` regenerates `docs/assets/img/console/conversations-*.png` and `conversation-*.png` from the worktree, and both are checked by eye against `prototype/C-rail.html`.
- [x] 9.2.3 `cd platform/console/ui && npm run demo` regenerates the landing recording and poster under `docs/assets/video/` from the worktree.
- [x] 9.2.4 `docs/getting-started.md`: every sentence naming the list, the unread switch or the detail tabs reads true against the new view.
- [x] 9.2.5 The site lint from `docs/CLAUDE.md` passes over every page touched, and the built site is looked at in both colour schemes.
