## 1. The build gate (design D1, D5)

- [ ] 1.1 Write `.github/scripts/review-build.sh <group>`: derives the recipe from the tree per the D1 table (go.mod → `go build ./... && go vet ./...`; `platform/console` → `npm ci && npm run build` in `ui/` first; `runtimes/claude|copilot` → `node --test`; `chart` → `helm lint chart && helm template chart >/dev/null`; anything else → built), prints the recipe it chose, and on failure writes the unbuilt reading `{component, unbuilt: <last 40 lines>, findings: [], changedNames: [], files: [], threads: [], unread: [<paths>]}` to `--out` and sets `built=false` on `$GITHUB_OUTPUT`. Verify: `review-build.sh docs` prints "no build" and exits 0; a fake go module with a syntax error yields `built=false` and a reading naming the component.
- [ ] 1.2 `.github/scripts/review-reading-check.py`: accept a reading carrying `unbuilt` (a string) with empty lists as valid, and reject `unbuilt` beside a non-empty `findings`. Verify: the check passes the unbuilt shape and prints `<group>: unbuilt`.
- [ ] 1.3 `claude-review.yml` `read` job: conditional `setup-go` / `setup-node` / `setup-helm` on the entry's kind and group, then `id: build` running `review-build.sh` — restored from the base branch with the other read-side scripts — before `claude -p`; the model step runs on `steps.build.outputs.built == 'true'`, and the reading upload takes the unbuilt reading otherwise. The build step's `env` holds no secret. Verify: `.github/tests/claude-review.test.sh` asserts the build step precedes the model step, the script is in the restore list, and the token is on the model step only.

## 2. Coverage: recorded on the pull request (design D2)

- [ ] 2.1 `review-prompt.py coordinator --coverage <file>`: write `{"sha": headSha, "paths": [...]}` from the validated readings — every `files[].path` minus `unread[]`, nothing from an unbuilt or absent component. Verify: a readings directory with one full reading, one with an unread file and one unbuilt yields exactly the read paths.
- [ ] 2.2 `review-post.py --coverage <file>`: append `<!-- claude-review-coverage {...} -->` as the summary's last line before posting; a coverage file that cannot be read or serialised is an error, never a summary posted without it. Verify: the posted body ends with the marker holding the sha and paths; a missing file fails the post with a named error.
- [ ] 2.3 `claude-review.yml` `consolidate`: write `coverage.json` before the model runs and hand it to `review-post.py`. Verify: the workflow test asserts the coverage step precedes the model step and the post reads the file.

## 3. The delta queue (design D3)

- [ ] 3.1 `review-input.py`: read the pull request's issue comments (`gh api --paginate`), take the coverage markers authored by the workflow, newest first, into `reviewedAt: path → sha`; decide per changed path per the D3 table (`git merge-base --is-ancestor`, `git diff --quiet <sha> HEAD -- <path>`); invalidate everything when `.claude/rules/**` or `openspec/changes/<name>/specs/**` differs between the recorded sha and head, or when `--full` is given; print the verdict per path with its reason. Verify: the log names every path as `read (since <sha|base>)` or `carried`.
- [ ] 3.2 `review-input.json` gains `since` per queued path, `carried: [{path, since, threads}]`, and `coverageInvalidated: "<reason>"|""`; the matrix holds only components with a read path; `queue[].paths` (the sibling list) keeps every changed path. Verify: a component whose paths are all carried produces no matrix entry; a read component's siblings include its carried paths.
- [ ] 3.3 `claude-review.yml`: `workflow_dispatch` input `full` (boolean, default false) passed as `--full`; the queue step's summary line states reads, carried and the invalidation reason. Verify: the workflow test asserts the input exists and reaches the program.

## 4. Readers and coordinator over the delta (design D4)

- [ ] 4.1 `review-prompt.py component`: each file carries `since`; `review-component.js` passes it and the reader message says `git diff -M <since>...HEAD -- <file>`; `.claude/agents/file-reviewer.md` names `since` where it named the base ref. Verify: the component args for a re-read file carry the recorded sha and for a new file carry the base ref.
- [ ] 4.2 `review-prompt.py coordinator`: the message gains `CARRIED PATHS (read at <sha>, unchanged since)` with each path's threads and the program's verdict `standing` for the unresolved ones, the counts for the coverage line, the invalidation reason, and each unbuilt component with its tail. Verify: the coordinator's message for an input with carried paths holds the block and the three numbers.
- [ ] 4.3 `.claude/agents/review-coordinator.md`: a carried path's threads are left as they are and counted as carried over; the reach search excludes only the read paths; the summary's third line `read: N of M changed files · K carried · J in unbuilt components`, the invalidation reason appended to it when set, and the `| unbuilt | <group> |` row. Verify: the role file states each, and the summary bound is restated as twelve VISIBLE lines.
- [ ] 4.4 `review-context.py component`: measure the diff from each file's `since`, not the base. Verify: the printed diff bytes for a re-read file are the delta's.

## 5. Unit tests

- [ ] 5.1 New `.github/tests/review-build.test.sh`: the recipe per group against a fake tree (a go module, a console-shaped directory, a Node runtime, `chart`, `docs`), the unbuilt reading on a failing build, the `built` output both ways; wired into `run.sh`. Verify: `.github/tests/run.sh` passes.
- [ ] 5.2 `review-input.test.sh`: the stubbed `gh` answers `api repos/*/issues/*/comments` with two markers (newer wins), and the fixture repository has commits so that one path is unchanged since its sha, one changed, one new, one recorded at a sha that is not an ancestor; a rules-file commit invalidates everything; `--full` invalidates everything; the carried list, the `since` per file and the matrix are asserted. Verify: `run.sh` passes.
- [ ] 5.3 `review-post.test.sh`: the marker is appended, a missing coverage file fails; `review-reading-check.test.sh`: the unbuilt shape passes and the mixed shape fails; `claude-review.test.sh`: the build step, the coverage step, the `full` input, the restore lists and the secret's placement. Verify: `run.sh` passes.
- [ ] 5.4 Run the whole suite and the two guards from the worktree: `.github/tests/run.sh`, `python3 .github/scripts/publication-guard.py`, `python3 .github/scripts/retired-vocabulary-guard.py`. Verify: all three exit 0.

## 6. E2E tests

- [ ] 6.1 Not applicable: nothing here is decided by a cluster — the change is a GitHub Actions workflow and the programs it runs. The live proof is the first review of this change's own pull request dispatched by hand (`gh workflow run claude-review.yml --ref change/review-built-delta -f number=<pr>`), then a push touching one file, whose summary must read the delta and carry the rest. Verify: the two summaries on the pull request state those numbers.

## 7. Documentation

### 7.1 Reference docs

- [ ] 7.1.1 `.claude/rules/worktree-delivery.md`, "The review found something": the build gate before a reading, the delta since the last review, the coverage marker on the pull request, `full=true` on a dispatch, and `unbuilt` beside `unreviewed`/`unread`. Verify: the section names all five.
- [ ] 7.1.2 `.claude/rules/gotchas.md`, "A review subagent under the action": one bullet recording that a reading is per file per DELTA now, so the per-reading measurements are per file read, not per push. Verify: the bullet is there and the numbers it qualifies still read correctly.
- [ ] 7.1.3 The delta spec is archived into `openspec/specs/automated-code-review/spec.md` by `/opsx:archive` on the branch; `openspec validate --all` passes. Verify: the command exits 0.

### 7.2 Adopter site

- [ ] 7.2.1 `CONTRIBUTING.md`, "Claude reviews the pull request": it builds each component first and reads none that fails, reads only what changed since the last review and carries the rest, and says both in its summary. No page under `docs/` describes the review, so the site carries nothing to update — stated here so the absence is a claim rather than an omission. Verify: `wc -l README.md` unchanged, and the paragraph names build, delta and `-f full=true`.
