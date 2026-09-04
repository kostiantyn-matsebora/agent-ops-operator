## 1. Baseline

- [x] 1.1 Record the BEFORE coverage for all sixteen components, read from
  `api/measures/component?metricKeys=coverage,lines_to_cover,uncovered_lines`
  (main branch, 2026-09-02). Verify: sixteen rows, one per
  `components.sh images` entry.

  | Component | Coverage | Lines to cover | Uncovered |
  |---|---|---|---|
  | signal-cron | 26.6% | 188 | 138 |
  | housekeeping | 39.4% | 203 | 123 |
  | scripts (gate-exempt) | 53.7% | 2382 | 1103 |
  | signal-telegram | 59.3% | 236 | 96 |
  | gateway-telegram | 60.1% | 178 | 71 |
  | runtime-claude | 63.8% | 443 | 182 |
  | egress-proxy | 65.0% | 618 | 216 |
  | context-sync | 67.1% | 432 | 142 |
  | runtime-ollama | 69.5% | 827 | 252 |
  | manager | 69.6% | 5958 | 1809 |
  | channel-telegram | 69.7% | 1116 | 338 |
  | runtime-copilot | 72.9% | 715 | 228 |
  | signal-alertmanager | 73.7% | 308 | 81 |
  | signal-ha | 73.7% | 1150 | 303 |
  | console | 76.0% | 7769 | 1891 |
  | signal-k8s-events | 78.7% | 1210 | 258 |

  **`Coverage` IS NOT `(lines to cover − uncovered) / lines to cover`,
  and `runtime-claude`/`runtime-copilot` are where that shows.** For every
  Go component the two agree to within rounding — SonarCloud's Go coverage
  report is line-only, so the composite metric collapses to the simple
  ratio. `runtime-claude` (63.8%) and `runtime-copilot` (72.9%) are the ONLY
  two rows where they diverge (58.9% and 68.1% by the naive ratio), and they
  are exactly the two components `node --test --experimental-test-coverage`
  measures, whose report includes BRANCH coverage alongside line coverage —
  which `coverage` composites in, per SonarCloud's own documented formula.
  The pattern isolating to precisely those two rows, and no others, is what
  makes this a toolchain difference rather than a mistyped figure: an actual
  transcription error would not land only on the two rows sharing one
  non-Go coverage tool.

  None clears the gate's 80% condition. Worked worst-first per `design.md`.

## 2. signal-cron (26.6% → 80%)

- [ ] 2.1 In `signals/cron/`, add tests closing the gap (~100 of 138
  uncovered lines must become covered). Target the scheduler and cron-parser
  branches first via `go tool cover -func=coverage.out | sort -k3 -n`.
  Verify: `go test -coverpkg=./... -coverprofile=coverage.out ./...` in the
  `golang:1.25` container reports ≥80% total.
- [ ] 2.2 Record the local before/after total in this task.

## 3. housekeeping (39.4% → 80%)

- [ ] 3.1 In `platform/housekeeping/`, add tests closing the gap (~82 of 123
  uncovered lines). The reclaim job's disk-scan and etcd-list stages
  (`invariants.md`'s "phase-blind listing") are the likely largest gaps —
  confirm from the coverage profile rather than assuming. Verify:
  `-coverpkg=./...` run reports ≥80% total.
- [ ] 3.2 Record the local before/after total in this task.

## 4. scripts (53.7% → 80%, gate-exempt but still covered)

- [ ] 4.1 In `.github/scripts/`, add or extend `.github/tests/*.test.sh`
  coverage closing the gap (~626 of 1103 uncovered lines) — the largest
  absolute gap in the org. Verify: the four-step Python coverage recipe in
  `build-test.md` reports ≥80% on `.github/coverage.xml`.
- [ ] 4.2 Record the local before/after total in this task.

## 5. signal-telegram (59.3% → 80%)

- [ ] 5.1 In `signals/telegram/`, add tests closing the gap (~49 of 96
  uncovered lines). Verify: `-coverpkg=./...` run reports ≥80% total.
- [ ] 5.2 Record the local before/after total in this task.

## 6. gateway-telegram (60.1% → 80%)

- [x] 6.1 In `gateways/telegram/`, add tests closing the gap (~35 of 71
  uncovered lines) — the `is_topic_message` classifier and offset persistence
  are the likely candidates. Verify: `-coverpkg=./...` run reports ≥80%
  total.

  Added `main_test.go` (poll's full lifecycle over real `httptest` servers,
  `loadConfig`'s credential discovery, `sleepCtx`, `route`'s callback-ack
  branch, `main()` itself stopped via a real SIGINT), `downstream_test.go`
  (marshal/request-construction errors, `Forward`'s ≥400 branch), and
  `telegram_errors_test.go` (`API`'s rejection/unreachable/malformed-JSON
  branches, `GetUpdates` malformed-payload branches). No production code
  changed; no dead code found.
- [x] 6.2 Record the local before/after total in this task.

  Before: 60.1% (SonarCloud) / 56.2% local. After: **92.5%** local
  (`-coverpkg=./... -coverprofile`, verified with `-race`, 27/27 tests pass).

## 7. runtime-claude (63.8% → 80%)

- [ ] 7.1 In `runtimes/claude/`, add `node --test` cases closing the gap
  (~93 of 182 uncovered lines). Verify:
  `node --test --experimental-test-coverage` reports ≥80% (noting the runtime
  reports coverage only for files the suite loads, per `build-test.md`).
- [ ] 7.2 Record the local before/after total in this task.

## 8. egress-proxy (65.0% → 80%)

- [ ] 8.1 In `platform/egress-proxy/`, add tests closing the gap (~92 of 216
  uncovered lines) across both subcommands (`install-redirect`, `proxy`).
  Verify: `-coverpkg=./...` run reports ≥80% total.
- [ ] 8.2 Record the local before/after total in this task.

## 9. context-sync (67.1% → 80%)

- [ ] 9.1 In `platform/context-sync/`, add tests closing the gap (~56 of 142
  uncovered lines) — the generation/symlink-swap and quiesced-vs-best-effort
  copy paths are the likely candidates. Verify: `-coverpkg=./...` run reports
  ≥80% total.
- [ ] 9.2 Record the local before/after total in this task.

## 10. runtime-ollama (69.5% → 80%)

- [ ] 10.1 In `runtimes/ollama/`, add tests closing the gap (~87 of 252
  uncovered lines) — this session's `sonar-ratings-baseline` work already
  added `repo_test.go` and `gitexec_test.go`; find the next-largest
  uncovered file from the profile rather than re-testing those. Verify:
  `-coverpkg=./...` run reports ≥80% total.
- [ ] 10.2 Record the local before/after total in this task.

## 11. manager (69.6% → 80%)

- [x] 11.1 In `platform/manager/`, add tests closing the gap (~617 of 1809
  uncovered lines — the largest gate-enforced gap in the org). Work package
  by package from `go tool cover -func`, favoring `internal/httpapi`,
  `internal/controller` and `internal/chat` where `coverage-across-packages`
  measured the deepest gaps. Verify:
  `KUBEBUILDER_ASSETS=... go test -coverpkg=./... -coverprofile=coverage.out ./...`
  reports ≥80% total.

  **Already closed by `sonar-ratings-baseline` (PR #145, merged 2026-09-04
  as part of the same session that raised ratings)** — its commit message
  states the module went 70.7% → 80.9% via a fuzzed deepcopy round-trip test
  plus the cognitive-complexity test extractions. No further work needed
  here; verified independently below.
- [x] 11.2 Record the local before/after total in this task.

  Before: 69.6% (SonarCloud, 2026-09-02). After: 81.5% local
  (`-coverpkg=./... -coverprofile` statement coverage, re-run on current
  `master`, 2026-09-04, envtest suite included, all packages passing). Clears
  the 80% gate.

## 12. channel-telegram (69.7% → 80%)

- [ ] 12.1 In `channels/telegram/`, add tests closing the gap (~115 of 338
  uncovered lines) — `sonar-ratings-baseline` already added
  `errorpaths_test.go`; find the next-largest uncovered file rather than
  re-testing it. Verify: `-coverpkg=./...` run reports ≥80% total.
- [ ] 12.2 Record the local before/after total in this task.

## 13. runtime-copilot (72.9% → 80%)

- [ ] 13.1 In `runtimes/copilot/`, add `node --test` cases closing the gap
  (~85 of 228 uncovered lines) — `vocabulary.js`'s permission-mapping
  branches and `continuity.js`'s resume ladder are the likely candidates.
  Verify: `node --test --experimental-test-coverage` reports ≥80%.
- [ ] 13.2 Record the local before/after total in this task.

## 14. signal-alertmanager (73.7% → 80%)

- [ ] 14.1 In `signals/alertmanager/`, add tests closing the gap (~19 of 81
  uncovered lines). Verify: `-coverpkg=./...` run reports ≥80% total.
- [ ] 14.2 Record the local before/after total in this task.

## 15. signal-ha (73.7% → 80%)

- [ ] 15.1 In `signals/ha/`, add tests closing the gap (~73 of 303 uncovered
  lines) — `sonar-ratings-baseline` already touched this component for
  ratings; find the next-largest uncovered file (the WebSocket reconnect
  ladder and the dwell re-check are candidates) from the profile. Verify:
  `-coverpkg=./...` run reports ≥80% total.
- [ ] 15.2 Record the local before/after total in this task.

## 16. console (76.0% → 80%)

- [ ] 16.1 In `platform/console/` (Go half), add tests closing the Go-side
  gap. Verify: `-coverpkg=./...` run reports the Go total.
- [ ] 16.2 In `platform/console/ui/` (TypeScript half), add `vitest` cases
  closing the UI-side gap — this project's coverage is the COMBINED Go+lcov
  number SonarCloud reports for one project, so both halves count toward the
  same 80%. Verify: `npm run test:coverage` reports the `src/**` total, and
  the combined figure (weighted by `lines_to_cover` from each report) is
  ≥80%.
- [ ] 16.3 Record the local before/after totals (both halves) in this task.

## 17. signal-k8s-events (78.7% → 80%)

- [ ] 17.1 In `signals/k8s-events/`, add tests closing the gap (~16 of 258
  uncovered lines — the smallest remaining gap in the org). Verify:
  `-coverpkg=./...` run reports ≥80% total.
- [ ] 17.2 Record the local before/after total in this task.

## 18. Unit tests

- [ ] 18.1 Run `go build ./... && go vet ./... && go test ./...` in every Go
  module touched above (`build-test.md`'s module loop). Verify: all pass,
  zero regressions in modules not otherwise touched by this change.
- [ ] 18.2 Run `node --test` in `runtimes/claude`, `runtimes/copilot`, and
  `npm run test:coverage` in `platform/console/ui`. Verify: all pass.
- [ ] 18.3 Run `.github/tests/run.sh` for any `.github/scripts/` test
  additions. Verify: exits 0.

## 19. E2E tests

- [ ] 19.1 Not applicable: this change adds tests against existing behavior
  only. No CRD field, RBAC rule, pod lifecycle, informer or context-continuity
  behavior changes, so nothing here is decided by a cluster. The live proof
  is each component's own SonarCloud `coverage` measure after this branch's
  changes merge to `master` — recorded per component's task above and
  re-confirmed in task 20.1.3 below.

## 20. Documentation

### 20.1 Reference docs

- [ ] 20.1.1 `docs/concepts.md` and `docs/contracts.md` describe CRD fields,
  semantics and contracts — this change adds tests against EXISTING
  behavior only (`proposal.md`'s Impact section), so neither page has
  anything this change makes untrue. Verify at archive time that no task
  above ended up changing production behavior; if one did, that page's
  section is updated here instead of left as this claim.
- [ ] 20.1.2 If any component's remaining gap required deleting dead code or
  adding a coverage-tool exclusion (rather than a test), record which
  component and why in this task — otherwise state "no exclusions were
  needed" here.
- [ ] 20.1.3 Record the AFTER coverage per component beside task 1.1's table,
  read from the first `master` analysis after this change's PR merges.
  Verify: sixteen rows, and every row is ≥80% except any recorded as an
  explicit, justified exception in task 20.1.2.

### 20.2 Adopter site

- [ ] 20.2.1 `CONTRIBUTING.md`, "Code analysis": if task 20.1.2 recorded any
  exclusion, add one sentence naming it and why; otherwise no wording there
  becomes false (it already states the 80% threshold and that a component
  under it is expected to be red), so leave it untouched and state that here.
  Verify: `wc -l README.md` is unchanged (this change touches no README
  section).
