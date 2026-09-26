## 1. The machine

- [x] 1.1 `.github/scripts/conveyor.py`: the station table, the loop table, and `standing_grant`, `fire`, `carry_fix`, `carry_archive`, `gate`, `guard`, `ending`, `check_is_work`, `recover`, `refresh`, `cap_for`, `count_marked`, `is_finished`. Verify: `.github/tests/conveyor.test.py` walks every state against every event and every fact combination of each decision
- [x] 1.2 `conveyor_io.py`: the shared fact gathering, failing closed on unreadable fire records and pull request lists. Verify: the adapter suites below read through it

## 2. The adapters

- [x] 2.1 `conveyor-state.py` takes events and writes one edit or none. Verify: `conveyor-state.test.sh`
- [x] 2.2 `carry.py` replaces `carry-grant.py` and `carry-from-pr.sh`, and `remote-implement.yml` calls it from `open` and `archive`. Verify: `carry.test.sh`, `review-dispatch.test.sh`
- [x] 2.3 `remote-implement.py` fires through the machine. Verify: `remote-implement.test.sh`
- [x] 2.4 `dispatch-gate.py` replaces the gate's shell in `review-dispatch.yml`. Verify: `dispatch-gate.test.sh`
- [x] 2.5 `land-dispatch.py` counts every round that ran, through one counter, and caps. Verify: `land-dispatch.test.sh`
- [x] 2.6 `autofix-guard.py --purpose ci|archive`, `failed-checks.py` reading failed steps, and `ci.yml` passing `--purpose ci`. Verify: `autofix-guard.test.sh`, `failed-checks.test.sh`, `dispute-answered.test.sh`
- [x] 2.7 `refresh-loop-state.py` and `recover-loop-state.py` send events. Verify: their suites

## 3. Unit tests

- [x] 3.1 `.github/tests/run.sh` passes, with `conveyor.test.py` and a suite per adapter

## 4. E2E tests

- [x] 4.1 Nothing here is decided by a cluster: every program runs on the runner against the platform's API, and the script suite stands `gh` in. Not applicable

## 5. Documentation

### 5.1 Reference docs

- [x] 5.1.1 `.claude/rules/worktree-delivery.md`, `remote-session.md` and `.github/routines/implement-issue.md`: the programs are named as they are now, and the rules say the machine owns the decisions
- [x] 5.1.2 `.claude/rules/gotchas.md`: the measurements on #248, #254 and #255 and why each earlier fix failed
- [x] 5.1.3 `docs/CHANGELOG.md` and `docs/diagrams/conveyor-lifecycle-implementation.mmd`: the behaviours, unreleased, and the drawn process

### 5.2 Adopter site

- [x] 5.2.1 Nothing on the adopter site describes the conveyor's rounds. Checked: `docs/index.md`, `docs/getting-started.md`, `docs/installation.md` and the integration pages name no fixing loop
