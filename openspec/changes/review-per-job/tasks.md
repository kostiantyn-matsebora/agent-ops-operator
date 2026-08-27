## 1. The composite action: pinned CLI, cached, and the base-branch restore

- [x] 1.1 Create `.github/actions/claude-cli/action.yml` (design D4): inputs `version` (default is the one place the version is written), `restore`, `base-ref`; a deterministic `npm` prefix, `actions/cache` keyed on the version, install on miss, `claude --version` asserted equal to the pin, then `git checkout <base> -- <path>` for each restore path present on base, with a `restored` output listing those that differed. Verify: the action parses (`python3 -c 'import yaml;yaml.safe_load(open(".github/actions/claude-cli/action.yml"))'`) and its version string appears nowhere else in `.github/`.

## 2. The reading validator

- [x] 2.1 Add `.github/scripts/review-reading-check.py`: reads the CLI's JSON envelope from a file, takes `result`, extracts the first JSON object in it, validates the reader's shape (required `component`, `findings[{path,line,claim}]`, `changedNames`, `threads[{id, verdict∈fixed|standing|gone|detached}]`), writes the reading to the given path or exits 1 naming what failed. Verify with `.github/tests/review-reading-check.test.sh`: a valid reading passes and is written; prose fails; a missing key fails; a bad verdict fails; JSON embedded in prose is extracted.

## 3. The workflow as four jobs

- [x] 3.1 Rewrite `.github/workflows/claude-review.yml` `queue` job (design D2): hygiene guards, then `review-input.py` (new; it and `review-prompt.py`, which assembles each role's message, are tested in `review-input.test.sh`) producing `queue` and `count` outputs and uploading `review-input.json`; `workflow_dispatch` with `number` and `dry_run` inputs, the PR resolved from the input on dispatch (design D6). Verify: `actionlint` or `python3 -c 'import yaml'` passes, and no step in `queue` runs `claude`.
- [x] 3.2 Add the `read` matrix job (design D3): `if: needs.queue.outputs.count != '0'`, `strategy.matrix.include: fromJSON(needs.queue.outputs.queue)`, `continue-on-error: true`, full-depth checkout, the action restored from base before `uses:`, the composite with `restore: .claude/agents/component-reviewer.md`, a Python step assembling the delegation message for `matrix.group` from `review-input.json`, `claude -p --agent component-reviewer --output-format json` with the reader's allowlist and the `claudeMdExcludes` settings, the validator, and upload of `reading-<group>.json`. Verify: the test in 5.1 asserts each element.
- [x] 3.3 Add the `consolidate` job (design D5): `needs: [queue, read]`, `if: always() && needs.queue.result == 'success' && !inputs.dry_run`, downloads the input and every reading artifact, assembles the coordinator's message with `null` for a missing reading, runs `review-coordinator` with `stream-json` to the execution artifact, then the existing summary-count gate, `.resolve-threads` touch and upload with `include-hidden-files`. Point `reconcile` at `needs: consolidate`. Verify: the test in 5.1 asserts the gate and the resolve-list upload live in `consolidate`.
- [x] 3.4 Delete `.claude/workflows/review-pr.js`. Verify: `git grep -n review-pr.js` returns only the archive under `openspec/changes/archive/` and the records in the rules files edited in section 6.

## 4. The roles

- [x] 4.1 `.claude/agents/component-reviewer.md`: the header comment names the matrix job rather than the script; `tools:` gains `Bash(git ls-files:*)`, `Bash(git ls-tree:*)`, `Bash(git cat-file:*)`; a stated list of what is unavailable (output redirection, paths outside the checkout, `helm`, `go`, `python3`, `npm`) with the instruction not to attempt them. Verify: the frontmatter parses and the test in 5.1 asserts the three git commands are present and no write-capable tool is.
- [x] 4.2 `.claude/agents/review-coordinator.md`: the header comment names the `consolidate` job; the delegation-message description says readings arrive as files assembled by the job. No change to the posting rules or the summary shape. Verify: `diff` of the body below the header shows only the delegation paragraph changed.

## 5. Tests

- [x] 5.1 Rewrite `.github/tests/claude-review.test.sh` (design D7): the three model jobs use `./.github/actions/claude-cli` and each restores the action from base in a step that precedes `uses:`; `queue` runs no `claude`; `read` reads `needs.queue.outputs.queue`, has `continue-on-error`, names `component-reviewer` in `restore` and in `--agent`; the reader's `--allowedTools` holds the git read commands and no `Write`/`Edit`; `consolidate` names `review-coordinator` and holds the summary gate and the resolve-list upload; `reconcile` needs `consolidate`; `workflow_dispatch` carries `number`; the `claudeMdExcludes` list matches today's six. Verify: `.github/tests/run.sh` passes.
- [x] 5.2 Add the case to `.github/tests/review-queue.test.sh` that the queue's JSON is valid as a matrix `include` list (every entry has `group`, `kind`, `paths`, and `paths` is a list). Verify: `.github/tests/run.sh` passes.
- [x] 5.3 Run the review by hand against an open pull request from the worktree's branch: push the branch, open the pull request, and dispatch `gh workflow run claude-review.yml --ref change/review-per-job -f number=<pr> -f dry_run=true`. Verify from the run: one `read` job per component, all started within the same minute, each uploading a reading or failing by name; then a non-dry run posts one summary with the count line and the "review actually ran" gate passes. Record the run's wall-clock in the tracking issue against #106's 10 min.

## 6. Documentation

### 6.1 Reference docs

- [x] 6.1.1 `CONTRIBUTING.md`: the "Claude reviews the pull request" paragraph describes the matrix — one job per component, consolidated once — and the by-hand path `gh workflow run claude-review.yml -f number=<pr>` with `dry_run`; no mention of a saved workflow or `/review-pr`. Verify: `git grep -n '/review-pr\|review-pr.js' CONTRIBUTING.md` is empty.
- [x] 6.1.2 `.claude/rules/worktree-delivery.md` "THE REVIEW FOUND SOMETHING" — the review is four jobs, the roles are the two files, the fan-out is the matrix; the by-hand command. Verify: the section names no script.
- [x] 6.1.3 `.claude/rules/gotchas.md` — the "THE REVIEW'S FAN-OUT IS A WORKFLOW SCRIPT" and "A BACKGROUND `Workflow` NEVER RUNS UNDER IT" entries are kept as the record and gain the sentence that the fan-out is an Actions matrix now, with the measured cap (`min(16, max(2, CPUs − 2))`, two on `ubuntu-latest`, PR #106) and why a job is the unit. Verify: the entries read as history, not as a current claim.
- [x] 6.1.4 `.claude/rules/retired-vocabulary.md` and `.github/retired-vocabulary.json` — add `review-pr.js` / `/review-pr` with `says` naming the workflow dispatch. Verify: `python3 .github/scripts/retired-vocabulary-guard.py` passes on the tree.
- [x] 6.1.5 `docs/CHANGELOG.md` is NOT touched — this is repository tooling, not a release. Verify: `git diff --stat docs/` is empty, and `python3 .github/scripts/publication-guard.py` passes.

### 6.2 Adopter site

- [x] 6.2.1 Confirm the site says nothing the change made untrue: `git grep -n 'review-pr\|claude-review\|component-reviewer\|review-coordinator' docs/` is empty (it was at proposal time), so no page changes. Verify: the grep is empty and this task records that.
