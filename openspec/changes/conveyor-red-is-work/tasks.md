## 1. The state machine

- [x] 1.1 Add `waiting` to `LOOP_STATES` and `end:waiting` to `LOOP_EVENTS` in `.github/scripts/conveyor.py`, with the transitions in design decision 4, and verify `conveyor.test.py`'s table test enumerates the new state and every listed transition
- [x] 1.2 Rework `ending()`: `clean` with a thread open or disputes remaining, and `disputed`, return `end:waiting`, while `no report`, `failed`, `timed out`, `stale patch` and `could not start` stay `end:stalled`. Add an `unaddressed` ending that continues below the cap. Verify with the exhaustive ending test in `conveyor.test.py`
- [x] 1.3 Reduce `check_is_work` to `skip` and `work`, deleting the `waiting` action and `guard_step` parameter, and verify the test named "without a named guard step nothing is waiting" is replaced by one asserting a `docs-task` failure on any step is work
- [x] 1.4 Remove the `ci` purpose from `guard()`, keeping `archive`, and verify `conveyor.test.py` rejects `ci` as an unknown purpose
- [x] 1.5 Move `unanswered_after_marker` and `carries_marker` from `autofix-guard.py` into `conveyor.py`, re-import them in `autofix-guard.py`, and verify `autofix-guard.test.sh` still passes unchanged
- [x] 1.6 Add `round_may_start(ci_concluded, review_concluded)` returning `start` or `defer` with the reason, and `refresh()`/`recover()` moving to `waiting` where they moved to `stalled` on an open thread. Verify with new cases in `conveyor.test.py`

## 2. The guard leaves CI

- [x] 2.1 Delete the `docs-task` step "No dispute the fixing loop posted waits unanswered" from `.github/workflows/ci.yml`, and verify `.github/tests/review-dispatch.test.sh`'s required-jobs reading still lists `docs-task` and the workflow parses
- [x] 2.2 Delete `.github/workflows/dispute-answered.yml`, `.github/scripts/rerun-ci-job.py`, `.github/tests/dispute-answered.test.sh` and `.github/tests/rerun-ci-job.test.sh`, and verify `git grep` finds no reference outside `docs/CHANGELOG.md` and the retired-vocabulary file
- [x] 2.3 Add `dispute-answered` and `rerun-ci-job` to `.github/retired-vocabulary.json`, each saying "a person's comment starts a round" as the replacement, and verify `retired-vocabulary-guard.py` passes over the tree
- [x] 2.4 In `failed-checks.py`, delete `guard_step_name`, `failed_steps` and the `waiting` output, and compute `consulted` as every required check run having `status: completed`. Verify `failed-checks.test.sh` gains a case where a queued check run yields `consulted: false`

## 3. Landing and endings

- [x] 3.1 In `land-dispatch.py`, route an `evidence` failure to `unaddressed` with its reason, post no thread reply for it, and list it in the summary's Unaddressed section. Verify `land-dispatch.test.sh` asserts no dispute marker is posted for a claimed fix whose patch misses the file
- [x] 3.2 Replace `Round.waiting_on_person` with a read of the collected work: true when every remaining item carries a dispute the fixing step made. Verify with a `land-dispatch.test.sh` case where the checks file carries no `waiting` key
- [x] 3.3 Make the `unaddressed` and `stale patch` endings dispatch `review-dispatch.yml` with `mode=all` when the round count is below the cap, after posting the summary, and give `land` `actions: write` in `review-dispatch.yml`. Verify the stubbed `gh` in `land-dispatch.test.sh` records the dispatch, and records none at the cap
- [x] 3.4 Add "head is red" or "head is green" to every summary from the collected checks, and verify a `land-dispatch.test.sh` case for each

## 4. The gate

- [x] 4.1 Accept a CI `workflow_run` conclusion of `success` in the gate's `if:` and its `workflow_run` branch under `conveyor:fix`, and verify `dispatch-gate.test.sh` starts a round on a green CI with an open review thread and marks mergeable with none
- [x] 4.2 Before any `mode=all` start, read the head's `ci-green` check run and review run, ask `round_may_start`, and on `defer` post one notice and exit `mode=none`. Verify `dispatch-gate.test.sh` covers a review completion with CI in progress, a grant placed mid-run, and both concluded
- [x] 4.3 Accept a non-bot comment on a pull request carrying `conveyor:fix` and `loop:waiting` as a `mode=all` start, keeping `/fix-accepted` as is, and verify `dispatch-gate.test.sh` starts on such a comment and ignores one on a running loop
- [x] 4.4 In `accepted-findings.py`, put a disputed thread back on the `--mode all` list when a person commented after the marker, reading `conveyor.unanswered_after_marker`. Verify `accepted-findings.test.sh` covers answered and unanswered disputes

## 5. The sweep

- [x] 5.1 Add `.github/workflows/conveyor-sweep.yml` on `schedule` every fifteen minutes and `workflow_dispatch`, one model-free job with `pull-requests: read` and `actions: write`, calling a new `.github/scripts/conveyor-sweep.py`. Verify the workflow's `on:` keys pass the suite's trigger-name test
- [x] 5.2 Write `conveyor-sweep.py`: list open pull requests carrying the fix label and `loop:waiting`, dispatch a `mode=all` round for each with no unanswered dispute, print one line per pull request. Verify a new `conveyor-sweep.test.sh` with a stubbed `gh` covers answered, resolved, unanswered and unlabelled

## 6. Vocabulary and labels

- [x] 6.1 Add `loop_labels.waiting: loop:waiting` to `.github/review-triage.json`, and verify `conveyor-state.test.sh` applies and removes it through `conveyor-state.py`
- [x] 6.2 Create the `loop:waiting` label on the repository with `gh label create`, colour and description beside the other loop labels, and verify `gh label list` shows it

## 7. Unit tests

- [x] 7.1 Run `.github/tests/run.sh` in this worktree and verify every script test passes, the new cases in 1.x to 6.x included
- [x] 7.2 Run `python3 .github/tests/conveyor.test.py` alone and verify the loop table test enumerates `waiting` in every row it enumerates the other states
- [x] 7.3 Run `python3 .github/scripts/retired-vocabulary-guard.py`, `publication-guard.py` and `.claude/scripts/rules_compliance.py` over every markdown file this change touched, and verify all three pass

## 8. E2E tests

- [x] 8.1 Nothing here is decided by a cluster. The change touches workflows and the scripts they call, all exercised by `.github/tests/run.sh` with a stubbed `gh`. No lane is added and the pack is not run

## 9. Documentation

### Reference docs

- [x] 9.1 Rewrite the loop section of `CONTRIBUTING.md`: the label list gains `loop:waiting` with its meaning, the `docs-task` re-run paragraph is deleted, a person's comment and the sweep are what resume a waiting loop, and a round that lands nothing continues. Verify by reading it against the merged specs
- [x] 9.2 Update `docs/testing.md`'s row naming `dispute-answered.yml` to say no check reads a person's answer, and verify no other row names the deleted files
- [x] 9.3 Update `docs/diagrams/conveyor-lifecycle-implementation.mmd`: a `waiting` node distinct from `stalled`, the comment and sweep paths into it, the self-continuing round, and the deleted `dispute-answered` node. Verify it renders on the site build
- [x] 9.4 Add a `docs/CHANGELOG.md` entry naming the deleted workflow and script, the new label and the new sweep, newest first
- [x] 9.5 Update `.claude/rules/worktree-delivery.md` (the label table and the review-found-something section), `.claude/rules/documentation.md` (the hook row), and add the #259 measurement to `.claude/rules/gotchas.md`. Verify `rules_compliance.py` passes on each

### Adopter site

- [x] 9.6 Re-read the landing page, `docs/introduction.md`, `docs/getting-started.md`, `docs/installation.md`, `docs/security.md` and every page under `docs/guides/` for a sentence about the fixing loop, disputes or `docs-task`, and record here which pages were read and that none needed a change, or fix the ones that did. Read: `docs/index.md`, `docs/introduction.md`, `docs/getting-started.md`, `docs/installation.md`, `docs/security.md`, `docs/guides/*.md`. Only `docs/security.md` names the loop, in "The fixing loop's push credential", and every sentence there still holds: the label, the read-only fixing step, the one bot and the gate's re-check are unchanged. No adopter page needed a change