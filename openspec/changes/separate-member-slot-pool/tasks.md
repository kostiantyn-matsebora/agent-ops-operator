## 1. Manager: two pools — backend-developer

- [x] 1.1 Add `MAX_ACTIVE_MEMBERS` (default 5, invalid → 5) beside `maxActiveConversations()` in `cmd/manager/main.go`, add a `MaxActiveMembers` field to the reconciler and the HTTP server, and verify `cmd/manager` tests cover default, set and invalid values, and that `MAX_RUNTIMES` sets only the conversations cap
- [x] 1.2 Add one pool classifier (`CausedBy` set means members, an unreadable conversation means conversations) and one helper splitting live pods and returning each pool's cap, and verify a unit test covers both classes and the unreadable case
- [x] 1.3 Scope `admit`, `waitingFIFO`, `evictableCount`, `evictIdleWorker` and the `createRuntimePod` re-check to the conversation's pool, and verify no remaining read of `MaxActiveConversations` outside the helper
- [x] 1.4 Scope the pod-deletion wake-up to the deleted pod's pool and its ceiling, and verify with a test that a freed member slot wakes a pending member
- [x] 1.5 Report both pools in `/status` (additive `members` sibling), relabel the slot gauges with `pool`, and verify `status_test.go` and the metrics tests

## 2. Chart — deployment-engineer

- [x] 2.1 Add `maxActiveMembers: 5` to `chart/values.yaml` with the comment, render `MAX_ACTIVE_MEMBERS` in `chart/templates/deployment.yaml`, and verify `helm template` shows both env vars at 5
- [x] 2.2 Update the ceiling alert text in `chart/templates/metrics.yaml` and any rule expression over the slot gauges for the `pool` label, and verify the chart render tests pass

## 3. Console — frontend-developer

- [ ] 3.1 Show both pools in the Queues reading in `platform/console/ui/src/model/queues.ts` and its view (ceiling detail names the key per pool), and verify with a vitest case plus a screenshot of the Queues page. PENDING: the screenshot step was skipped — no reachable manager backend in this sandbox. Vitest coverage proves the rendering instead. Also wired `platform/console/queues.go`/`manager.go` to forward the manager's `members` field — not named in this task's own file list, but required for the UI to receive real data.

## 4. Unit tests — testing-specialist

- [x] 4.1 Controller tests for the scenarios in the conversation-capacity delta: full conversations pool admits a member, full members pool admits a conversation, sixth member pends, eviction stays in the pool, a younger member is not held behind an older conversation, and the integration suite (envtest) passes
- [x] 4.2 Add a controller test for nested coordinators: a member Coordinator with an ended run holds a members slot, the pool is otherwise full of busy members, a child of that member is admitted by evicting the idle parent, and a chain longer than the ceiling completes
- [x] 4.3 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager`, the chart render tests, and `npm run test` plus `npm run test:e2e` in `platform/console/ui` — all green (e2e needed a local browser-executable override for this sandbox's pinned Playwright version, never committed)

## 5. System tests

- [ ] 5.1 Extend a lane in `platform/manager/test/system/` (the capacity/replicas lane) so a chain of nested Coordinator members longer than `MAX_ACTIVE_MEMBERS` completes without stalling, and a Coordinator root with more members than the members ceiling, beside a full conversations pool, admits members up to `MAX_ACTIVE_MEMBERS` and no more, then run the pack against this working copy's tree. Written and compiles/vets clean under `-tags system`. Not executed here — no docker/k3d in this sandbox — dispatched to `system-smoke.yml` instead

## 6. Documentation

### 6.1 Reference docs

- [x] 6.1.1 `docs/concepts.md` Capacity section, `docs/configuration.md` Capacity table and the ephemeral-storage note, `docs/contracts.md` `/status`, `docs/console.md` ceiling row, and the `.claude/rules/invariants.md` row naming `MAX_ACTIVE_CONVERSATIONS`, which gains `MAX_ACTIVE_MEMBERS` beside it
- [x] 6.1.2 `docs/CHANGELOG.md` entry marking the capacity change as breaking-by-capacity with the upgrade step, and the metric relabel
- [x] 6.1.3 Re-run `python3 .github/scripts/docs-generate.py` and `python3 .github/scripts/retired-vocabulary-guard.py`, and verify both pass

### 6.2 Adopter site

- [x] 6.2.1 `docs/installation.md` values row, `docs/getting-started.md` and the landing page where a slot count is stated, and the Coordinator guide under `docs/guides/` naming that a member holds a slot of its own pool
- [x] 6.2.2 Re-run `npm run screenshots` and `npm run demo` in `platform/console/ui` for the Queues change, and run `python3 .github/scripts/publication-guard.py`
