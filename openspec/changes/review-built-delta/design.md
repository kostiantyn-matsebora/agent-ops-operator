## Context

See proposal.md — Why. What shapes the approach:

- **The review is four jobs on the `pull_request` event**, independent of
  `ci.yml`. `queue` is a program over `gh pr diff --name-only` and the review
  threads; `read` is a matrix job per component running `claude -p` over the
  saved workflow `review-component.js`; `consolidate` runs the coordinator
  and posts through `review-post.py`; `reconcile` resolves threads.
- **Everything that decides what the review does is restored from the base
  branch** before it runs, and a pull request editing `claude-review.yml`
  is not reviewed until it merges. A new script joins those lists or it is
  the pull request's own code deciding its review.
- **The review's memory is the pull request.** The spec's "A finding already
  made is not made again" requirement forbids a second store: threads are
  handed to the next run as context. Any record of what was reviewed has to
  live on the pull request too.
- **A reading is per file**: `review-component.js` hands each file reader
  `git diff -M <base>...HEAD -- <file>`, the file, its threads and the rules
  routed to its path, and merges the readings; a file with no reading is
  `unread`. The coordinator holds readings and threads, no rules, and reaches
  outside the change with `git grep` excluding the changed paths.
- **CI knows how each component builds**: `modules` (`go build`, `go vet`,
  with the console's UI bundle built first because its Go build embeds it),
  `node-runtimes` (`node --test`), `chart` (`helm lint` and a render). The
  same recipe is what "built" means here.

## Goals / Non-Goals

**Goals:**

- No model token is spent on a component whose build fails, and the summary
  says so by name with the compiler's own words.
- No model token is spent re-reading a file no commit touched since it was
  last read, on the same base, against the same rules.
- Nothing the coordinator could find before becomes unfindable: threads,
  reach, cross-file references all keep their scope.
- Every decision is a program's, stated in the job log and the summary; the
  model decides nothing about what is read.

**Non-Goals:**

- Waiting for `ci.yml`'s verdict, or coupling to its job names (rejected
  below).
- Reviewing only changed HUNKS. The unit stays the file: a reader reads the
  whole file it is on, and judges rules against the file, not the hunk.
- Any change to what a reader looks for, to the finding shape, to triage or
  to the dispatch.
- A store outside the pull request — no cache, no artifact carried across
  runs, no label.

## Decisions

### D1. The `read` job builds its component itself, before the model

Each `read` job runs `.github/scripts/review-build.sh <group>` as a step
before `claude -p`. The script derives the recipe from the tree, the way
`components.sh` derives the component list:

| Group | Recipe | Why this one |
|---|---|---|
| a directory with `go.mod` | `go build ./... && go vet ./...` | CI's `modules` job, minus the tests |
| `platform/console` | `npm ci && npm run build` in `ui/`, then the Go recipe | its Go build embeds `ui/dist` and fails without it |
| `runtimes/claude`, `runtimes/copilot` | `node --test` | CI's `node-runtimes` job; there is no compile step, the suite is the build |
| `chart` | `helm lint chart && helm template chart >/dev/null` | CI's `chart` job's first two steps; the permutation matrix is CI's, not a prefilter's |
| any other group | nothing — built | prose and configuration; a docs group never compiles |

- **On failure** the step writes the reading itself:
  `{"component": g, "unbuilt": "<last 40 lines of output>", "findings": [],
  "changedNames": [], "files": [], "threads": [], "unread": [<every path>]}`,
  uploads it and SKIPS the model step (`if: steps.build.outputs.built ==
  'true'`). The job is green — an unbuilt component is a fact about the pull
  request, not about the review — and CI's own red check is what says the
  build failed.
- **The build step holds no secret.** `CLAUDE_CODE_OAUTH_TOKEN` is on the
  model step's `env` alone; the build runs the pull request's code (its
  `package.json` scripts, its `go generate` if any) under `contents: read`
  and nothing else. The tests are NOT run for a Go module: `go test` executes
  the pull request's tests for a gate whose only question is "does this
  compile", and the answer to that is `go build` plus `go vet`.
- **The script is restored from the base branch** with the other queue-side
  scripts, so a pull request cannot declare itself built.
- **Cost:** the build is repeated outside CI — runner minutes on a public
  repository, cached by `setup-go` / `setup-node` under the same keys CI
  uses. The setup steps run conditionally on the group's kind so a docs job
  installs nothing.

**Alternatives rejected.**

- *Read CI's verdict for the component from the head sha's check runs.* The
  review starts with CI, so it would poll — a runner idling per component
  for as long as CI's matrix takes — and it couples to `ci.yml`'s job names
  (`modules (platform/manager)`), which nothing else reads; a cancelled or
  superseded CI run reads as "unbuilt". Every one of those is a silent way to
  review nothing.
- *Trigger the review from `workflow_run` after `ci.yml`.* Loses the
  `pull_request` context the queue, the guards and the dispatch are built on,
  and every review waits for the slowest CI job (e2e, image scans) to learn
  whether `signals/cron` compiled.
- *Have the file reader run the build.* The reader is a model with read-only
  tools by design; a build is not a reading, and the token the gate exists to
  save would be spent starting the session.

### D2. Coverage is a marker in the summary comment, written by the program

`review-post.py` appends one line to the summary it posts:

```
<!-- claude-review-coverage {"sha":"<head sha>","paths":["<path>", ...]} -->
```

- `paths` is computed by `review-prompt.py coordinator --coverage
  coverage.json` from the READINGS, not from the model's answer: every path
  in a reading's `files[]`, minus its `unread[]`; an unbuilt component
  contributes nothing. The consolidate job writes that file before the
  model runs and hands it to `review-post.py`, exactly as `repo`/`number`
  are the job's facts and not the model's.
- Hidden in rendered markdown; inside the twelve-line bound because the
  bound is on what a reader sees. The gate still reads `summaryPosted`.
- **A dry run posts nothing and so records nothing** — correct: nothing was
  reviewed on the pull request's record.

**Why on the pull request and not an artifact or a cache.** The existing
requirement: continuity is context, not stored state, because a second store
is wrong precisely when a run failed part-way. A marker posted in the same
call as the summary is either there with the summary or absent with it.

**Why per path and not per component or one sha for the run.** A run in
which one component was unbuilt, one reading failed and one file was unread
has reviewed a set of PATHS, and only that set may be skipped next time. One
sha for the run would skip the unread file forever; per component would
re-read a twenty-file component for one unread file.

### D3. The queue decides per path, and carries the rest

`review-input.py` reads the review's own summary comments (`gh api
repos/{repo}/issues/{n}/comments --paginate`, the marker present, author the
workflow's), builds `reviewedAt: path → sha` newest-first, and for each path
of `gh pr diff --name-only` decides:

| Condition | Verdict |
|---|---|
| no marker names the path | READ, `since` = base |
| the marker's sha is not an ancestor of head (`git merge-base --is-ancestor`) — a rebase or force-push | READ, `since` = base |
| the path changed between that sha and head (`git diff --quiet <sha> HEAD -- <path>`) | READ, `since` = that sha |
| what a file is judged against changed since that sha: any `.claude/rules/**` or the change's delta specs (`openspec/changes/<name>/specs/**`) differs between the sha and head | READ, `since` = base, for EVERY path |
| a dispatch with `full=true` | READ, `since` = base, for every path |
| otherwise | CARRIED |

- The matrix holds only components with at least one READ path; a component
  whose paths are all carried has no job. `entries[].paths` carry `since`
  per file; the component's `all_paths` (the reader's sibling list) still
  holds every changed path, carried included, so a reference to a carried
  sibling is recognisable.
- `review-input.json` gains `carried: [{path, since, threads}]` and
  `coverageInvalidated: "<reason>"|""`, and the queue step prints the
  decision per path — the log line is the audit.
- The `since` for a READ path is the sha it was last read at, so the reader's
  diff is the delta; the whole file is still read, so a finding can land on
  an older line, and its anchor is the nearest line of the pull request's
  diff as today.

### D4. The reader diffs from `since`; the coordinator's reach still sees carried files

- `review-component.js` passes each file's `since` and the reader runs
  `git diff -M <since>...HEAD -- <file>` in place of `<base>...HEAD`. The
  role file says so. The rule and spec reading is unchanged.
- The coordinator's message gains `CARRIED PATHS (read at <sha>, unchanged
  since)` with their threads. Their UNRESOLVED threads are `standing` by
  construction — the code they concern has not changed since the review
  that left them — and the coordinator says nothing about them and counts
  them as carried over; their resolved threads count as dismissed or resolved
  as today. The program states the verdict in the message; the coordinator
  does not infer it.
- The coordinator's `git grep` reach EXCLUDES ONLY THE READ PATHS. A carried
  file is then a consumer outside the readings, found and read by the one
  bounded read the coordinator already makes — the previous review's
  files are cross-checked against this delta's removed and renamed names
  without a reading of their own.
- The summary's fixed shape gains a third line:
  `read: N of M changed files · K carried · J in unbuilt components`, and the
  nothing-new table gains `| unbuilt | <group> |` beside `unreviewed`. The
  numbers are in the coordinator's message; it copies them.

### D5. `review-reading-check.py` accepts the unbuilt shape as a reading

A reading with `unbuilt` set and every list empty is valid, so the merge in
`review-prompt.py coordinator` handles it as any other: the component is
handed to the coordinator with `unbuilt: "<tail>"`, never as `null`.
`unreviewed` (the model produced nothing usable) and `unbuilt` (the model was
never run) stay distinct, because they ask the author for different things.

## Risks / Trade-offs

- [A file whose finding depends on another file that changed — a caller
  carried, a callee read] → the callee's reading declares the changed name;
  the coordinator's reach search now includes carried files and reads the
  consumer. That is the existing cross-check applied to a smaller exclusion.
- [The coverage marker grows with the pull request — a hundred paths is a
  few kilobytes in a hidden line] → GitHub's comment bound is 65 536
  characters; the marker holds paths and one sha; a pull request past that
  bound has a bigger problem than its review. The posting program truncates
  nothing: a marker that cannot be written is an error, never a silently
  shorter one that skips too little.
- [A base-branch merge into the branch changes files the pull request never
  touched] → the decision runs over `gh pr diff --name-only` only; a file the
  merge brought in is not the pull request's and is not queued.
- [A rule file change on the BASE branch, merged in, changes what carried
  files would be judged against] → the invalidation checks `.claude/rules/**`
  between the recorded sha and head, whichever side brought the change.
- [The build script and CI drift — CI adds a step the prefilter lacks] → the
  script's test asserts each recipe against the tree, and `ci-green` remains
  the gate: the prefilter answers "worth reading", CI answers "mergeable".
  A component the prefilter passes and CI fails is reviewed and red, which
  is today's behaviour.
- [`go build` in the read job pays module download per job] → `setup-go`'s
  cache keyed on the module's `go.sum`, the key CI already warmed.
- [The landed dispatch commit (`Fixed in <sha>`) starts no review, and the
  next hand push reviews the delta including it] → unchanged from today, and
  the delta is exactly where a fix is re-read against its thread.

## Open Questions

None that change the specs or the tasks. The `node --test` recipe for a Node
runtime is a suite rather than a compile; if it proves slow the recipe can
become `node --check` over the runtime's files without a spec change.
