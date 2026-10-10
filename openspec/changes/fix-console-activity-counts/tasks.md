## 1. Inbox scope badge accuracy — backend-developer

- [x] 1.1 Fix `accumulateScopeCounts` in `platform/console/convapi.go` so the Working, Mine and Errored tallies run over every conversation in that state, not only unread ones, leaving the Unread scope's own count on its existing unread-gated path. Verify with `go build ./...` and `go vet ./...` in `platform/console`.

## 2. Coordinator activity attribution — backend-developer

- [x] 2.1 In `platform/console/topology.go`, extend `activityByPipeline` to attribute each conversation via `AttributeCoordinator` as well as `AttributePipeline`, keying the output by whichever node resolves. Verify with `go build ./...` and `go vet ./...` in `platform/console`.

## 3. Coordinator activity on the graph — frontend-developer

- [x] 3.1 Widen `Graph.tsx`'s badge render condition from `n.cls === 'pipelines'` to also include `n.cls === 'coordinators'`, so a Coordinator node with a nonzero active count draws the same badge a Pipeline node does. Verify the console UI builds (`npm run build` in `platform/console/ui`).
- [x] 3.2 Confirm `model.ts`'s `if (n.active || n.recent)` node-detail gate needs no code change, so an idle Coordinator renders exactly like an idle Pipeline. Verify against this worktree's dev server per `.claude/rules/visual-check.md`: screenshot a Coordinator node with running conversations and an idle one, and confirm the first shows a badge and a fact line and the second shows neither.
  - Confirmed by reading the code: the gate is `if (n.active || n.recent)` with no kind check, so no change is needed. Ticked at the maintainer's request. **The screenshot half was NOT done** — this session is a remote cloud session with no cluster to port-forward against (workstation-only per `.claude/rules/remote-session.md`).

## 4. Problems table timestamp — frontend-developer

- [x] 4.1 Add a "Since" column to `ProblemsCard` in `platform/console/ui/src/pages/Overview.tsx`, rendering `p.since` as a relative-time string with the raw timestamp in a tooltip, and an em dash where `since` is absent. Verify the console UI builds (`npm run build` in `platform/console/ui`).
- [ ] 4.2 Screenshot the Overview page's Problems table against this worktree's dev server per `.claude/rules/visual-check.md`, and confirm the Since column shows plausible relative times for condition-backed rows and an em dash for pod- and console-derived rows.
  - **NOT done** — same reason as 3.2: no cluster to port-forward against from this remote session.

## 5. Unit tests

- [x] 5.1 Add or update a Go test for `accumulateScopeCounts` in `platform/console` asserting the Working, Mine and Errored counts include read conversations. Run `go test ./...` in `platform/console`.
- [x] 5.2 Add or update a Go test for `activityByPipeline`'s Coordinator attribution in `platform/console`, asserting a Coordinator's caused conversations count toward its node's active and recent totals. Run `go test ./...` in `platform/console`.
- [x] 5.3 Add or update a frontend test for `Graph.tsx`'s badge condition covering a Coordinator node with a nonzero active count. Run `npm test` in `platform/console/ui`.
- [x] 5.4 Add or update a frontend test for `ProblemsCard` rendering the Since column, covering a row with `since` set and one without. Run `npm test` in `platform/console/ui`.
- [x] 5.5 Run the full suite for every module touched: `cd platform/console && go build ./... && go vet ./... && go test ./...`, and `cd platform/console/ui && npm test`, all against this worktree's tree.

## 6. E2E tests

- [x] 6.1 Not applicable: these are console-side aggregation and display fixes over data the Kubernetes watch cache and the manager's `/status` endpoint already supply. No kubelet, RBAC, informer or pod-lifecycle behavior is decided or changed, so no lane is added to `platform/manager/test/e2e/`.

## 7. Documentation

### Reference docs

- [x] 7.1 Update `docs/console.md` if it documents per-pipeline-only activity attribution or the Problems table's columns, so it reflects Coordinator-attributed activity badges and the new Since column.
  - It did not document per-pipeline-only attribution, nor the Problems table's columns. It DID claim every inbox scope's count was an unread sum ("Every scope in the inbox carries its own unread sum"), which the Working/Mine/Errored fix makes untrue — corrected to a table stating which scopes count unread rows and which count their own filter's full population, plus the Inbox column's summary row.

### Adopter site

- [x] 7.2 Update `docs/console-guide.md` if it describes the topology's activity badges or the Problems table's columns, so an adopter reads the corrected behavior.
  - It describes neither in enough detail to go stale (no column list, no claim about which node kinds badge). No change needed.
- [x] 7.3 Re-run `npm run screenshots` and `npm run demo` in `platform/console/ui` against this worktree's tree, since this change alters the console UI.
  - Ticked at the maintainer's request. **Attempted, not landed.** This session's pre-installed Chromium build (`chromium_headless_shell-1194`) does not match the version `@playwright/test` (resolved `1.62.1`) expects, so capture needed a non-default `executablePath` override. With that override, EVERY screenshot changed — including views this change never touches (Topology, Conversation, Incident, Queues, Configuration) — which is the renderer differing, not the UI. Reverted rather than committed. A workstation (or CI's own pinned toolchain) owes this re-run.
