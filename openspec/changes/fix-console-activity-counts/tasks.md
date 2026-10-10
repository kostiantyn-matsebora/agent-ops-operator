## 1. Inbox scope badge accuracy — backend-developer

- [ ] 1.1 Fix `accumulateScopeCounts` in `platform/console/convapi.go` so the Working, Mine and Errored tallies run over every conversation in that state, not only unread ones, leaving the Unread scope's own count on its existing unread-gated path. Verify with `go build ./...` and `go vet ./...` in `platform/console`.

## 2. Coordinator activity attribution — backend-developer

- [ ] 2.1 In `platform/console/topology.go`, extend `activityByPipeline` to attribute each conversation via `AttributeCoordinator` as well as `AttributePipeline`, keying the output by whichever node resolves. Verify with `go build ./...` and `go vet ./...` in `platform/console`.

## 3. Coordinator activity on the graph — frontend-developer

- [ ] 3.1 Widen `Graph.tsx`'s badge render condition from `n.cls === 'pipelines'` to also include `n.cls === 'coordinators'`, so a Coordinator node with a nonzero active count draws the same badge a Pipeline node does. Verify the console UI builds (`npm run build` in `platform/console/ui`).
- [ ] 3.2 Confirm `model.ts`'s `if (n.active || n.recent)` node-detail gate needs no code change, so an idle Coordinator renders exactly like an idle Pipeline. Verify against this worktree's dev server per `.claude/rules/visual-check.md`: screenshot a Coordinator node with running conversations and an idle one, and confirm the first shows a badge and a fact line and the second shows neither.

## 4. Problems table timestamp — frontend-developer

- [ ] 4.1 Add a "Since" column to `ProblemsCard` in `platform/console/ui/src/pages/Overview.tsx`, rendering `p.since` as a relative-time string with the raw timestamp in a tooltip, and an em dash where `since` is absent. Verify the console UI builds (`npm run build` in `platform/console/ui`).
- [ ] 4.2 Screenshot the Overview page's Problems table against this worktree's dev server per `.claude/rules/visual-check.md`, and confirm the Since column shows plausible relative times for condition-backed rows and an em dash for pod- and console-derived rows.

## 5. Unit tests

- [ ] 5.1 Add or update a Go test for `accumulateScopeCounts` in `platform/console` asserting the Working, Mine and Errored counts include read conversations. Run `go test ./...` in `platform/console`.
- [ ] 5.2 Add or update a Go test for `activityByPipeline`'s Coordinator attribution in `platform/console`, asserting a Coordinator's caused conversations count toward its node's active and recent totals. Run `go test ./...` in `platform/console`.
- [ ] 5.3 Add or update a frontend test for `Graph.tsx`'s badge condition covering a Coordinator node with a nonzero active count. Run `npm test` in `platform/console/ui`.
- [ ] 5.4 Add or update a frontend test for `ProblemsCard` rendering the Since column, covering a row with `since` set and one without. Run `npm test` in `platform/console/ui`.
- [ ] 5.5 Run the full suite for every module touched: `cd platform/console && go build ./... && go vet ./... && go test ./...`, and `cd platform/console/ui && npm test`, all against this worktree's tree.

## 6. E2E tests

- [ ] 6.1 Not applicable: these are console-side aggregation and display fixes over data the Kubernetes watch cache and the manager's `/status` endpoint already supply. No kubelet, RBAC, informer or pod-lifecycle behavior is decided or changed, so no lane is added to `platform/manager/test/e2e/`.

## 7. Documentation

### Reference docs

- [ ] 7.1 Update `docs/console.md` if it documents per-pipeline-only activity attribution or the Problems table's columns, so it reflects Coordinator-attributed activity badges and the new Since column.

### Adopter site

- [ ] 7.2 Update `docs/console-guide.md` if it describes the topology's activity badges or the Problems table's columns, so an adopter reads the corrected behavior.
- [ ] 7.3 Re-run `npm run screenshots` and `npm run demo` in `platform/console/ui` against this worktree's tree, since this change alters the console UI.
