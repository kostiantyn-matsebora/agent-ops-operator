## 1. The label vocabulary

- [x] 1.1 Add `station_labels.stalled` to `.github/review-triage.json`, and extend its comment to name what sets `station:stalled` and why. Verify: `python3 -c 'import json;json.load(open(".github/review-triage.json"))'` still parses
- [x] 1.2 Create the `station:stalled` label in the repository by hand (`gh label create station:stalled --description "Issue. State, not a grant: a merge landed but nothing carries the line onward."`), matching the color of its `station:*` siblings. Verify: `gh label list` shows it

## 2. The archive station marks a stalled merge

- [x] 2.1 `carry-grant.py`, finding no `conveyor:run` on an opsx-lane issue for `--station archive`, sets `station:stalled` instead of leaving the label untouched. The plain lane and `--station fix` are unaffected. Verify: the three new cases in `carry-grant.test.sh` below

## 3. Documentation the change made untrue

### 3.1 Reference docs

- [x] 3.1.1 `.claude/rules/worktree-delivery.md`: the `station:<x>` label row's value list gains `stalled`, with why it is set
- [x] 3.1.2 `.claude/rules/remote-session.md`: its `station:<x>` row already delegates the value list to `worktree-delivery.md` rather than restating it, so nothing there needs an edit

### 3.2 Adopter site

- [x] 3.2.1 Confirm nothing on the site mentions the conveyor's station labels (`grep -rn "station:" docs/*.md docs/guides/*.md`) — none does, so no page needs a change

## 4. Unit tests

- [x] 4.1 `.github/tests/carry-grant.test.sh`: three new cases, covering an opsx-lane `--station archive` call with no `conveyor:run` (sets `station:stalled`), a plain-lane one (unaffected), and a `--station fix` call (unaffected)
- [x] 4.2 `.github/tests/run.sh` passes in full
- [x] 4.3 `python3 .github/scripts/publication-guard.py` and `python3 .github/scripts/retired-vocabulary-guard.py` pass on this worktree

## 5. E2E tests

- [x] 5.1 Not applicable: nothing here is decided by a cluster. The change is one script and one vocabulary entry, and `docs/testing.md` places those in the script suite.

## 6. Documentation

- [x] 6.1 Both reference-doc edits above are the whole of what this change makes untrue, and are done as part of section 3
