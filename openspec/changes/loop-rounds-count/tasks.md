## 1. A timed-out round counts

- [ ] 1.1 `land-dispatch.py`: the `--fix-timed-out` ending posts the round marker, reports the rounds used, and at the cap names `conveyor:keep-going`. `--fix-failed` stays uncounted. Verify: `.github/tests/land-dispatch.test.sh` gains a case where five timed-out rounds reach the cap and the sixth is refused, and one where a failed fix still counts nothing

## 2. The running-round question leaves CI

- [ ] 2.1 `autofix-guard.py`: a `--disputes-only` flag that asks the dispute question alone. The hook keeps the default. Verify: `.github/tests/autofix-guard.test.sh` gains a case where a queued dispatch run refuses the default and passes disputes-only, and one where an unanswered dispute refuses both
- [ ] 2.2 `ci.yml`, job `docs-task`: the guard step passes `--disputes-only`, and its step name says what it asks. Verify: `.github/tests/review-dispatch.test.sh` (or the workflow suite that parses `ci.yml`) asserts the flag is present

## 3. The loop's own bookkeeping starts no round

- [ ] 3.1 `failed-checks.py`: read the failed job's steps and exclude a `docs-task` job whose only failed step is the guard step, with a stated reason in its output. Verify: `.github/tests/failed-checks.test.sh` gains a case with a stubbed jobs listing, one where `docs-task` failed on the guard step (excluded) and one where it failed on the tasks-file step (kept)
- [ ] 3.2 `land-dispatch.py`: a round whose work list is empty because every failed check was excluded ends as "nothing to do" and names the unanswered dispute. Verify: the land test's empty-work-list case names the dispute

## 4. Unit tests

- [ ] 4.1 `.github/tests/run.sh` passes, the four suites above included

## 5. E2E tests

- [x] 5.1 Nothing here is decided by a cluster: every program runs on the runner against the platform's API, and the script suite stands `gh` in. Not applicable

## 6. Documentation

### 6.1 Reference docs

- [ ] 6.1.1 `.claude/rules/worktree-delivery.md`: the loop bullets say a timed-out round counts, that `docs-task` asks the dispute question alone, and that a red made only of the guard starts no round
- [ ] 6.1.2 `.claude/rules/gotchas.md`: the measurement on #248 — the cycle, the six replies, and why removing the grant by hand was the wrong fix
- [ ] 6.1.3 `docs/CHANGELOG.md`: the three behaviours, unreleased

### 6.2 Adopter site

- [ ] 6.2.1 Nothing on the adopter site describes the conveyor's rounds. Checked: `docs/index.md`, `docs/getting-started.md`, `docs/installation.md` and the integration pages name no fixing loop
