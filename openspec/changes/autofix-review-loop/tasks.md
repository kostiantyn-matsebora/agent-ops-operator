## 1. The re-trigger spike (design D4) — decides D4 and D7 before anything is built on them

- [x] 1.1 On a throwaway pull request from this worktree's branch, push a commit with `GITHUB_TOKEN` from a scratch job, then `gh workflow run ci.yml` (with a temporary `workflow_dispatch` input checking out `refs/pull/<n>/head`) and `gh workflow run claude-review.yml -f number=<n>`; record in `design.md` D4 whether the merge box shows `ci-green` satisfied on that head and whether SonarCloud's own check attaches
- [x] 1.2 Write the outcome as a decision in `design.md` — self-dispatch, or the App — and strike the other; the remaining tasks in sections 4 and 6 follow that decision
- [ ] 1.3 The credential: the spike ruled out self-dispatch (D4, measured), and the App is replaced by a WRITE DEPLOY KEY read by `land` alone; `docs/security.md` records what holds it and what it cannot do. **OWNER'S STEP, not done by the session:** `ssh-keygen -t ed25519 -N '' -f autofix && gh repo deploy-key add autofix.pub --allow-write --title autofix-land && gh secret set AUTOFIX_DEPLOY_KEY < autofix && shred -u autofix autofix.pub` — until then a labelled round lands and the summary says the loop cannot go on

## 2. Consent: the label and the gate (D1, D2)

- [x] 2.1 `.github/review-triage.json`: add `approve_label: "autofix"` with its own `_comment` paragraph stating that it is CHANGE-level consent, who may place it, and that per-thread consent is unchanged without it
- [x] 2.2 `review-dispatch.yml`: add `pull_request: [labeled]` and `workflow_run: [claude-review] completed` triggers; `gate` outputs `mode` (`threads` | `all`), the pull request number and head sha for each event shape, and refuses a fork in every mode
- [x] 2.3 `gate`, label event: check the sender's permission via the collaborators API; on a non-writer REMOVE the label and post the refusal comment; record the approver's login for D6
- [x] 2.4 `gate`, `workflow_run` event: read the label off the pull request at that moment; absent → the run ends as skipped, visibly
- [x] 2.5 Create the `autofix` label on the repository (`gh label create`) with a description that says what placing it does

## 3. The two-source work list (D3)

- [x] 3.1 `accepted-findings.py --mode all`: every unresolved review-authored thread without the dispute marker; per-thread mode unchanged; each item carries `source: review`
- [x] 3.2 New `.github/scripts/sonar-issues.py`: open issues per component project for the pull request from `api/issues/search`, keyed `sonar:<issueKey>`, carrying rule, message, file, line, `source: sonar`; project keys derived from `components.sh` exactly as `sonar-scan/action.yml` derives them; detects "no analysis for this head" and reports it as a flag rather than an empty list
- [x] 3.3 `collect`: in `all` mode merge both into the one work-list JSON `fix` consumes; in `threads` mode unchanged
- [x] 3.4 `.github/publication-allowlist.json`: `sonarcloud.io` was already allowed by #127; nothing to add, the guard is clean

## 4. Fix or dispute, land, re-trigger, bound (D4, D5, D6)

- [x] 4.1 The fixer role prompt: the fix-or-dispute contract — every item yields `{id, action, reason}`; a Sonar item is fixed in code only; never touch the service
- [x] 4.2 `land-dispatch.py`: dispute replies in threads with the `<!-- autofix:disputed -->` marker, one pull-request comment for disputed Sonar keys, resolve ONLY `fixed` items whose patch landed
- [x] 4.3 `land-dispatch.py`: round counting from `<!-- autofix:round N -->` markers on its own comments; `MAX_ROUNDS` env; the four ending conditions from D6 and the ONE summary comment mentioning the approver
- [x] 4.4 `land`: the re-trigger per the spike — self-dispatch of `ci.yml` and `claude-review.yml`, or the App-token push; in `threads` mode the existing "push again" comment stays
- [x] 4.5 `ci.yml`'s workflow-trigger assertion for `review-dispatch.yml`: extend the pinned trigger set; assert `fix` carries no secret in either mode

## 5. The archive refusal (D8)

- [x] 5.1 New `.github/scripts/autofix-guard.py`: fail-open; with the label present, refuse while a `review-dispatch` run is in progress for the pull request or while a disputed thread has no later human comment
- [x] 5.2 `.claude/hooks/require-docs-task.sh` calls it after `docs-task-guard.py`; CI's `docs-task` job calls it too
- [x] 5.3 `.claude/skills/openspec-apply-change/SKILL.md`: after the pull request exists, ASK the owner whether the change is approved for automatic fixing and, on their word, `gh pr edit <n> --add-label autofix`; `openspec-archive-change/SKILL.md` names the refusal

## 6. SonarCloud's gate becomes required (D7)

- [x] 6.1 Per the spike's outcome: either `sonar (<component>)` waits on `api/qualitygates/project_status?pullRequest=` and fails on `ERROR`, or Sonar's own check is added to branch protection — and `ci-green` reports it either way
- [x] 6.2 `sonarcloud-analysis` (#127) archived on master (#134) and master is merged into this branch, so the `code-quality-analysis` delta modifies a published requirement

## 7. Unit tests

- [x] 7.1 `.github/tests/accepted-findings.test.sh`: `--mode all` lists every open review thread and skips a thread carrying the dispute marker; `threads` mode fixtures unchanged
- [x] 7.2 New `.github/tests/sonar-issues.test.sh` against a fixture response: keys, fields, the no-analysis flag, project-key derivation from the component list
- [x] 7.3 `.github/tests/land-dispatch.test.sh`: dispute replies posted and not resolved, Sonar disputes in one comment, round counter, each of the four endings produces exactly one summary mentioning the approver, stale patch lands nothing
- [x] 7.4 New `.github/tests/autofix-guard.test.sh`: fail-open cases, refusal on a running round, refusal on an unanswered dispute, allow when answered
- [x] 7.5 `.github/tests/claude-review.test.sh` / the `review-dispatch.yml` assertions: the trigger set, `fix` holds no secret, the label name in the gate expression matches `review-triage.json`
- [x] 7.6 `.github/tests/run.sh` passes in full

## 8. E2E tests

- [x] 8.1 Nothing here is decided by a cluster — the change is workflow scripts, a label and a hook; the live proof is the spike (1.1, recorded in D4 and `gotchas.md`) and the first labelled pull request once the credential exists

## 9. Documentation

- [x] 9.1 Reference docs: `.claude/rules/worktree-delivery.md` — the triage table gains the label row and the dispute row, the "landed commit has no CI" paragraph is bounded to unlabelled pull requests, the archive refusal is named; `.claude/rules/gotchas.md` — the token-push caveat bounded the same way, and the spike's result recorded as a measured fact; `CONTRIBUTING.md` — the review section and the SonarCloud gate now being required; `docs/CHANGELOG.md` — the gate becomes required; `docs/security.md` — the push credential (if the App), what holds it, what cannot; `.claude/rules/documentation.md` hook table — the second check `require-docs-task.sh` runs
- [x] 9.2 Adopter site: nothing an adopter of the operator reads changes — this is the contributor process; confirmed by re-reading `docs/index.md`, `introduction.md`, `getting-started.md`, `installation.md` for any mention of the review or SonarCloud, and stated here rather than left unlisted
