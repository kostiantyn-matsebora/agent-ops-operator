## 1. The vocabulary and the state script

- [x] 1.1 Add `station_labels` and `loop_labels` to `.github/review-triage.json`, with a comment saying they are STATE and grant nothing, and verify `python3 -c 'import json;json.load(open(".github/review-triage.json"))'` still parses
- [x] 1.2 Write `.github/scripts/conveyor-state.py` (`--repo --target <n> --station <x> | --loop <x>`): adds the named label, removes its siblings, exits 0 on every `gh` failure with a notice. Verify: with a stubbed `gh` that one call records exactly one add and the sibling removes
- [x] 1.3 Create the nine state labels in the repository by hand (`gh label create station:<x> --description "State, not a grant: ..."`, same for `loop:<x>`) and verify `gh label list` shows them

## 2. The fixing round runs, and reports when it cannot

- [x] 2.1 Add `allowed_bots: github-actions` to the `fix` job's `claude-code-action` step in `.github/workflows/review-dispatch.yml`, with the comment explaining the measured refusal on #220 and why not `*`. Verify: the workflow parses (`python3 -c 'import yaml;yaml.safe_load(open(...))'`)
- [x] 2.2 Let `land` run on `needs.fix.result == 'failure'`: substitute an empty patch and report as the skipped case does, and pass `--fix-failed` with the fix job's run URL. Verify: by reading the rendered `if:` and the substitution step
- [x] 2.3 Add `--fix-failed` to `land-dispatch.py`: one summary ending `fixing step failed` naming the run and the approver, no round counted, no item disputed, and the loop label set to `stalled`. Verify: with the script suite's stubbed `gh` that the summary is posted and no thread is touched
- [x] 2.4 Set the loop label at the transitions: `running` from `gate` when a round starts, `stalled` from `land` on disputes-only, no-report and fixer-failed endings, `capped` at the cap. Verify: each path in `land-dispatch.test.sh` records the expected `conveyor-state.py` call

## 3. The review verdict is read live, and a person's answer is what re-evaluates it

- [x] 3.1 Remove the "The review leaves nothing open" step from `claude-review.yml`'s `reconcile` job, and update `review-not-clean.py`'s docstring to say where it now runs. Verify: `claude-review.test.sh` still passes and pins the step's absence
- [x] 3.2 `ci.yml`'s `review-clean` job asks only whether the review ran (`review-is-green.py`), and no check reports the review's thread state. Verify: `review-clean.test.sh` pins the job's steps and that no workflow STEP runs `review-not-clean.py` as a check (its one caller is `carry-from-pr.sh`, for state, see 3.5)
- [x] 3.3 RETIRED in the follow-up: `rerun-review-clean.py` was written and removed, because the event it served does not exist. Verify: the file is gone
- [x] 3.4 RETIRED in the follow-up: `review-thread.yml` on `pull_request_review_thread` was refused by the platform (`Unexpected value 'pull_request_review_thread'`, a failed no-job run on every push). Removed, and `review-clean.test.sh` pins that no workflow names that event. Verify: `gh run list --workflow=review-thread.yml` shows nothing after the merge
- [x] 3.5 Set `loop:mergeable` and `station:merge` from `carry-from-pr.sh` (called by `remote-implement.yml`'s `open` job on a green `ci`) only when `review-not-clean.py` finds no review thread open. Verify: `carry-from-pr.test.sh`, both the clean and the open-thread case
- [x] 3.6 Add `.github/workflows/dispute-answered.yml` on `issue_comment` and `pull_request_review_comment` (created, non-bot, not a dispatch) with `actions: write`, re-running through `.github/scripts/rerun-ci-job.py` the FAILED `docs-task` job of the head's own `ci` run on a pull request carrying `conveyor:fix`. A run in progress, a missing run, a job that did not fail and a 403 each exit 0 with a notice. Verify: `rerun-ci-job.test.sh` and `dispute-answered.test.sh`
- [x] 3.7 Correct a stale `loop:mergeable` when a later review completes with findings open, on a pull request the loop is not driving (measured on #226): `.github/scripts/refresh-loop-state.py`, called from the gate's `mode=none` path. Verify: `refresh-loop-state.test.sh`

## 4. The archive station has an actor

- [x] 4.1 `remote-implement.py` accepts `archive_label`. A sender of `github-actions[bot]` is accepted only when `run_label` stands on the issue and its placer can push (share `label_placement` and `permission` with `carry-grant.py` through a small `grant.py` module). The fire record is per station (`<!-- remote-implement:fired:archive -->`, the implement marker unchanged). Verify: each case in `remote-implement.test.sh`
- [x] 4.2 `remote-implement.yml`'s `fire` job also fires on `conveyor:archive`. Verify: `review-dispatch.test.sh`'s pin on the fire `if:` is updated and passes
- [x] 4.3 Write `.github/routines/archive-change.md`: validate the number, find the change by `.github-issue`, confirm its pull request merged and no archive pull request is open, take or recreate `change/<name>`, `openspec validate --all`, `openspec archive <name> --yes`, `opsx-issue.sh phase <name> archived`, commit as `docs(openspec): archive <name>, folding its delta specs into the published contract`, push, `gh pr create` with `Closes #<n>` and no label, stop. Verify: by reading it against `worktree-delivery.md`'s archive rules
- [x] 4.4 Add the station switch at the top of `implement-issue.md`: an issue carrying `conveyor:archive` sends the session to `archive-change.md`. Verify: by reading
- [x] 4.5 `carry-from-pr.sh` reads `Closes #<n>` where `Refs #<n>` is absent for station `fix`, and for station `archive` treats a merged pull request saying `Closes` as the line's end: `station:done`, nothing carried. Verify: in `carry-grant.test.sh` (or a new `carry-from-pr.test.sh`) with a stubbed `gh`
- [x] 4.6 Set `station:implement` from `remote-implement.py` on fire, `station:fix` from `carry-grant.py --station fix`, `station:archive` from `carry-grant.py --station archive`, and `station:done` from the `archive` job on a plain-lane merge. Verify: each records the expected `conveyor-state.py` call in its test
- [x] 4.7 Update the label table comments in `.github/review-triage.json` and the descriptions of the five `conveyor:` labels where they say a person archives. Verify: `gh label list` and the file agree

## 5. Live proof, after the merge

- [ ] 5.1 After this change merges, open a throwaway plain-lane issue asking for a one-line change, place `conveyor:run`, and verify the station label moves implement → fix → merge → done with no further label placed by a person, and the loop label reads running then mergeable
- [ ] 5.2 On #220, push once (master merged into its branch) so the merged workflows run on it, and verify `ci-green` turns green with the review's thread resolved, the loop label reads `mergeable` and #51's station `merge`. A re-run of the old review run is not possible: its artifact expired after a day

## 6. Unit tests

- [x] 6.1 `.github/tests/run.sh` passes with the new and changed tests: `conveyor-state.test.sh`, `carry-from-pr.test.sh`, `review-clean.test.sh`, `rerun-ci-job.test.sh`, `dispute-answered.test.sh`, `refresh-loop-state.test.sh`, and the extended `land-dispatch`, `remote-implement`, `carry-grant`, `review-dispatch` and `claude-review` tests, each pinning the behaviour its section above names
- [x] 6.2 `python3 .github/scripts/publication-guard.py`, `python3 .github/scripts/retired-vocabulary-guard.py` and `python3 .github/scripts/docs-generate.py --check` pass on this worktree

## 7. E2E tests

- [x] 7.1 Not applicable: nothing here is decided by a cluster. Every changed behaviour is a workflow, a script or an instruction file, and `docs/testing.md` places those in the script suite.

## 8. Documentation

### 8.1 Reference docs

- [x] 8.1.1 `.claude/rules/worktree-delivery.md`: the review section says no check carries the thread question and why, and the label table gains the two state vocabularies and the archive station's actor
- [x] 8.1.2 `.claude/rules/remote-session.md`: the label table gains `conveyor:archive` starting a session, the archive routine, and the per-station fire record
- [x] 8.1.3 `.claude/rules/gotchas.md`: the #201 entry gains the second measurement, the action's bot allowlist refusing every carried round on #220
- [x] 8.1.4 `CONTRIBUTING.md`: the conveyor section describes the archive station, the state labels and what a person does on a stalled loop
- [x] 8.1.5 `docs/testing.md`: what `review-clean` decides, and that the threads are the merge box's live question
- [x] 8.1.6 `docs/diagrams/conveyor-lifecycle.mmd` and `conveyor-lifecycle-implementation.mmd`: committed, the implementation diagram redrawn without the defect nodes and with the state labels
- [x] 8.1.7 `.github/review-triage.json`'s comment block describes the state vocabularies

### 8.2 Adopter site

- [x] 8.2.1 `docs/security.md`: the fixing step's consent paragraph names the bot allowlist as a bound, and states that the archive station is a session under the standing instruction
- [x] 8.2.2 Confirm the landing page, `introduction.md`, `getting-started.md`, `installation.md` and the guides mention nothing this change made untrue (`grep -rn conveyor docs/*.md docs/guides/`)
