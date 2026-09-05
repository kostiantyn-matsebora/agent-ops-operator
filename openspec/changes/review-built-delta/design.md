## Context

See proposal.md — Why. What shapes the approach:

- **The review is four jobs on the `pull_request` event**, independent of
  `ci.yml`. `queue` is a program over `gh pr diff --name-only` and the review
  threads; `read` is a matrix job per component; `consolidate` runs the
  coordinator and posts through `review-post.py`; `reconcile` resolves
  threads. Today `read` runs one `claude -p` per component — a session that
  reads nothing and runs the saved workflow `review-component.js`, which
  starts two `file-reviewer` QUEUE READERS, each one context that reads its
  rules once and then its files one after another, holding its files'
  threads.
- **Everything that decides what the review does is restored from the base
  branch** before it runs, and a pull request editing `claude-review.yml`
  is not reviewed until it merges. A new script joins those lists or it is
  the pull request's own code deciding its review. A hand dispatch is exempt
  from the workflow-file guard and runs the branch's copy of the workflow
  FILE — but every program is still restored from the base, so a change to
  the programs can be exercised by hand only after it merges; before that a
  dispatch hands the new workflow the old programs.
- **The review's memory is the pull request.** The spec's "A finding already
  made is not made again" requirement forbids a second store: threads are
  handed to the next run as context. Any record of what was reviewed has to
  live on the pull request too.
- **The coordinator holds readings and threads, no rules**, and reaches
  outside the change with `git grep` excluding the changed paths. It is the
  only writer.
- **CI knows how each component builds**: `modules` (`go build`, `go vet`,
  with the console's UI bundle built first because its Go build embeds it),
  `node-runtimes` (`node --test`), `chart` (`helm lint` and a render). The
  same recipe is what "built" means here.
- **What a reader's context costs, measured from the tree** (bytes over four
  as tokens): the role file and CLAUDE.md are ~9 KB; the rules routed to a
  code path are 62–82 KB, 15–20 k tokens, of which invariants, wiring and
  terminology are 67 KB in every code path's set; a docs path gets 57 KB.
  Then each file plus its diff — a manager controller is 61 KB, the chart
  helpers 49 KB, `concepts.md` 88 KB; the last manager pull request's diff
  averaged 5.5 KB per file. A queue reader keeps all of it: on that pull
  request, 36 files over two readers, each reader ended past 100 k tokens,
  re-sent every turn.
- **Six facts about the CLI, settled locally on 2026-09-05 with `claude -p`,
  the review's model and effort, before this design was written** — because
  a day was once lost restructuring the review before isolating the
  variable (`gotchas.md`):
  1. `agent({agentType: 'fork'})` in a workflow script is refused: the type
     is not in the registry under `-p`.
  2. The Agent tool's `subagent_type: fork` from the model's turn under `-p`
     is refused with the same message. The fork exists in an interactive
     session and not here.
  3. A skill with `context: fork` runs under `-p` ("forked execution") and
     INHERITS NOTHING — asked for a nonce the parent had just read, it
     answered UNKNOWN. A fork with `agent: <project role>` runs as that
     role, its system prompt and its tools.
  4. One such fork with a 35 KB fixed body created 9.4 k cache tokens; three
     forks created 9.7 k. The body is written to cache once and read by the
     others.
  5. **Three separate `claude -p` PROCESSES with the same 35 KB in
     `--append-system-prompt`: the first created 22.9 k cache tokens and
     cost $0.096; the second and third created 3.6 k, read 37.8 k, and cost
     $0.023 each.** The prompt cache is server-side, keyed on the prefix
     bytes, and crosses processes.
  6. Under `-p` the workflow runtime's concurrent-agent pool is
     `min(16, max(2, cpus − 2))`, two on the runner, with no override — the
     measurement already in `gotchas.md`. A shell loop has no such pool.

## Goals / Non-Goals

**Goals:**

- No model token is spent on a component whose build fails, and the summary
  says so by name with the compiler's own words.
- Every read of a file is an INDEPENDENT sample: a reader holds no thread and
  no previous finding, and nothing read for another file.
- The rules are paid once per job and read from cache by every further
  reader, instead of once per reader as fresh input and again on every turn
  of a growing queue.
- A file stops being read when independent reads stop finding anything, and
  never before — a file no push touched is re-read until its reads are quiet.
- Nothing the coordinator could find before becomes unfindable: threads,
  reach, cross-file references all keep their scope.
- Every decision is a program's, stated in the job log and the summary; the
  model decides nothing about what is read, when a file is finished, or what
  is a duplicate of a thread it did not see.

**Non-Goals:**

- Claiming one read finds everything. It does not; the design makes the
  reads independent and cheap, and stops when they go quiet.
- Several readers per file per run (`N` shadows). Measured as a cost that
  multiplies the whole first pass — twenty files, three readers, sixty
  processes — for coverage the convergence rule reaches across the pushes a
  pull request already makes. Stated here so the knob is not added by
  default; if wanted later it is a loop count in the job.
- Waiting for `ci.yml`'s verdict, or coupling to its job names (rejected
  below).
- Reviewing only changed HUNKS. The unit stays the file: a reader reads the
  whole file it is on, and judges rules against the file, not the hunk.
- Any change to the finding shape, to triage or to the dispatch.
- A store outside the pull request — no cache directory, no artifact carried
  across runs, no label.

## Decisions

### D1. The `read` job builds its component itself, before the model

Each `read` job runs `.github/scripts/review-build.sh <group>` as a step
before any reader. The script derives the recipe from the tree, the way
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
  uploads it and SKIPS every reader (`if: steps.build.outputs.built ==
  'true'`). The job is green — an unbuilt component is a fact about the pull
  request, not about the review — and CI's own red check is what says the
  build failed.
- **The build step holds no secret.** `CLAUDE_CODE_OAUTH_TOKEN` is on the
  reader step's `env` alone; the build runs the pull request's code (its
  `package.json` scripts, its `go generate` if any) under `contents: read`
  and nothing else. The tests are NOT run for a Go module: `go test` executes
  the pull request's tests for a gate whose only question is "does this
  compile", and the answer to that is `go build` plus `go vet`.
- **The script is restored from the base branch** with the other read-side
  scripts — the `read` job's restore list, since that is the job that runs
  it — so a pull request cannot declare itself built. On the one pull
  request that INTRODUCES the script the base has no copy, and the restore
  loop's existing `git cat-file -e || continue` runs the pull request's own;
  that hole is one pull request wide, and every new review script has passed
  through the same one.
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

### D2. A reading is one file in one process, with the rules as a cached system prefix

The `read` job runs, for each file in its entry, one `claude -p` from the
shell — `xargs -P $REVIEW_READERS` over the entry's paths, four wide by
default; the width is API-bound, not a processor pool — with:

- `--append-system-prompt "$(review-prompt.py reader-system --group g)"`:
  the `file-reviewer` role's body, then the FULL TEXT of every rule file
  `review-rules.py` routes to the entry's paths (the union, so the bytes are
  identical for every file of the component), then the change's delta specs.
  The program emits it once per job to a file; every process reads the same
  bytes.
- the prompt from `review-prompt.py reader --group g --path p`: the path,
  `since`, the base ref, the names of the component's other changed files
  (carried ones included), and nothing else.
- `--json-schema` for one FILE READING (`path`, `findings`, `declares`,
  `references`), `--allowedTools` the read-only set the role names,
  `--model`, `--effort`, `--settings` as today.

Each process writes `readings/<slug>/<path-slug>.json`; the job then runs
`review-reading-check.py` over the directory, which merges the file readings
and the verdicts (D3) into the component reading the coordinator already
expects, naming as `unread` every path with no valid file. The component
reading's SHAPE is unchanged, so the coordinator's input is unchanged.

**Why a process and not the queue reader, the fork or a skill.**

| Shape | Rules loaded | Accumulates | Independent per file | Available under `-p` |
|---|---|---|---|---|
| queue reader (today) | once per reader, fresh, then re-sent per turn | the whole queue | no — later files see earlier ones, and its threads | yes |
| subagent per file | once per file, fresh | no | yes | yes |
| Agent-tool fork | inherited | no | yes | **no** (fact 2) |
| `context: fork` skill | in the skill body, cached across forks (fact 4) | no | yes (inherits nothing, fact 3) | yes |
| **one process per file** | in the system prompt, cached across processes (fact 5) | no | yes | yes |

The skill and the process have the same economics; the process has no layer
between the job and the reader — no component session whose one turn must
not end early (`gotchas.md`, #74/#77), no saved workflow, no runtime pool of
two (fact 6), and the loop is the job's shell, which is where this review
already keeps every plan it must not drop. The `review-component.js`
workflow is deleted rather than kept beside it: two mechanisms for one
reading is the drift `structure.md` names.

- **The rules go in the system prompt, not the prompt**, so the prefix shared
  across processes is as long as possible: Claude Code's own system prompt,
  the role, the rules, the specs — then the file-specific prompt. The 3.6 k
  tokens each later process still created (fact 5) are the CLI's per-process
  tail, not ours.
- **The role file stays the definition** (`.claude/agents/file-reviewer.md`),
  read by the program for its body — one file per role, restorable from the
  base, as the guarded-definition requirement says. It stops being a
  subagent definition and its `tools:` line becomes the `--allowedTools`
  the program passes.
- **Cache lifetime is minutes**, longer than a job. The processes of one job
  run seconds apart. A job for a component of one file pays the rules once
  as it does today; every file after the first is the saving.
- **Cost, measured:** at the review's model and effort a reader's fixed
  context is ~$0.10 for the first process of a job and ~$0.02 for each after,
  plus the file and its diff, which are never shared and never were.

### D3. A reading is blind; threads are judged apart; the coordinator dedups

- **The reader holds no thread and no previous finding.** A reader handed
  the threads tends to agree with them — it is the reviewer reading the last
  reviewer's notes before the file — and the review's rounds were measured to
  keep finding new things on unchanged files, so the rounds are worth having
  only if each is independent. The blind reader is what makes "read until
  quiet" (D5) mean anything.
- **A VERDICT process per file with unresolved threads**, after the blind
  read, from `.claude/agents/thread-verdict.md`: handed the threads (id,
  path, line, body, resolved), the file and its diff from `since`, returning
  `{threads: [{id, verdict}]}` in the existing vocabulary — `fixed`,
  `standing`, `gone`, `detached` (with the new location as a finding of the
  same claim). It is meant to be primed. Resolved threads are handed to
  nobody: a person's dismissal and a fix already recorded are history, as
  today. The program settles what it can first: a thread whose anchor lines
  are outside the file's current length is `gone` without a model.
- **"Not made again" is enforced at the posting, by the coordinator**, which
  holds every thread already: a blind finding on a path and claim an OPEN
  thread already states is folded into that thread's count (`carried over`),
  one a DISMISSED thread states is dropped and counted (`dismissed`), and
  the rest are posted. A finding that matches a thread the verdict pass
  called `fixed` is posted — the fix did not hold, and that is new.
- **The verdict pass costs one process per file with open threads**, small:
  the same cached system prefix minus the rules (the verdict role needs
  none), the file and the threads. On a pull request with no threads it
  costs nothing.

**Alternative rejected.** *The same reader, threads revealed after it has
written its reading.* One process is one prompt; the reading is committed in
the same turn as everything the process saw. Two prompts is two processes,
and then the second is the verdict pass.

### D4. Coverage is a marker in the summary comment, written by the program

`review-post.py` appends one line to the summary it posts:

```
<!-- claude-review-coverage {"sha":"<head sha>","paths":{"<path>":{"quiet":N},...}} -->
```

- `paths` is computed by `review-prompt.py coordinator --coverage
  coverage.json` from the READINGS, not from the model's answer: every path
  in a reading's `files[]`, minus its `unread[]`; an unbuilt component
  contributes nothing. The consolidate job writes that file before the
  model runs and hands it to `review-post.py`, exactly as `repo`/`number`
  are the job's facts and not the model's.
- `quiet` is the path's count of consecutive reads that added no new
  finding. `review-post.py` computes it at the moment it knows: the
  previous marker's value for that path (carried through
  `review-input.json` as `quietAt: path → N`, zero where none) plus one if
  it posted no inline finding on that path this run, else zero. A finding
  folded into an existing thread is not new and does not reset it.
- Hidden in rendered markdown; inside the twelve-line bound because the
  bound is on what a reader sees. The gate still reads `summaryPosted`.
- **A dry run posts nothing and so records nothing** — correct: nothing was
  reviewed on the pull request's record.

**Why on the pull request and not an artifact or a cache.** The existing
requirement: continuity is context, not stored state, because a second store
is wrong precisely when a run failed part-way. A marker posted in the same
call as the summary is either there with the summary or absent with it.

**Why per path.** A run in which one component was unbuilt, one process
failed and one file was unread has reviewed a set of PATHS, and only that set
may be counted. One sha for the run would skip the unread file forever; per
component would re-read a twenty-file component for one unread file.

### D5. The queue decides per path — read until quiet, then carry

`review-input.py` reads the review's own summary comments (`gh api
repos/{repo}/issues/{n}/comments --paginate`, the marker present, author the
workflow's), builds `readAt: path → sha` and `quietAt: path → N` from the
newest marker naming each path, and for each path of `gh pr diff
--name-only` decides, with `K = REVIEW_QUIET_READS` (workflow variable,
default 1):

| Condition | Verdict |
|---|---|
| a dispatch with `full=true` | READ, `since` = base, for every path |
| any `.claude/rules/**` or the change's delta specs (`openspec/changes/<name>/specs/**`) differs between a recorded sha and head | READ, `since` = base, for EVERY path — what a file is judged against changed |
| no marker names the path | READ, `since` = base |
| the marker's sha is not an ancestor of head (`git merge-base --is-ancestor`) — a rebase or force-push | READ, `since` = base |
| the path changed between that sha and head (`git diff --quiet <sha> HEAD -- <path>`) | READ, `since` = that sha |
| unchanged, and `quiet < K` | READ, `since` = that sha — a read that adds nothing is what earns the carry |
| unchanged, and `quiet ≥ K` | CARRIED |

- **K = 1 means: the first time an independent read of an unchanged file
  comes back with nothing new, believe it.** That is the moment a person
  calls a file reviewed today. K = 2 asks once more before believing it,
  one extra reading per file. The marker's history will say which was
  needed; nobody knows that number today, which is why it is a variable.
- The matrix holds only components with at least one READ path; a component
  whose paths are all carried has no job. `queue[].paths` (the sibling list)
  still holds every changed path, carried included, so a reference to a
  carried sibling is recognisable.
- `review-input.json` gains `since` and `quiet` per queued path, `carried:
  [{path, since, quiet, threads}]`, and `coverageInvalidated:
  "<reason>"|""`; the queue step prints the decision per path with its
  reason — the log line is the audit.
- The `since` for a READ path is the sha it was last read at, so the reader's
  diff is the delta; the whole file is still read, so a finding can land on
  an older line, and its anchor is the nearest line of the pull request's
  diff as today.

### D6. Carried files keep their place in the cross-check

- The coordinator's message gains `CARRIED PATHS (read at <sha>, quiet N)`
  with their threads. Their UNRESOLVED threads are `standing` by
  construction — the code they concern has not changed since the read that
  left them — and the coordinator says nothing about them and counts them as
  carried over; their resolved threads count as dismissed or resolved as
  today. The program states the verdict in the message; the coordinator
  does not infer it.
- The coordinator's `git grep` reach EXCLUDES ONLY THE READ PATHS. A carried
  file is then a consumer outside the readings, found and read by the one
  bounded read the coordinator already makes — the previous review's files
  are cross-checked against this delta's removed and renamed names without a
  reading of their own.
- The summary's fixed shape gains a third line:
  `read: N of M changed files · K carried · J in unbuilt components`, with
  the invalidation reason appended when set, and the nothing-new table gains
  `| unbuilt | <group> |` beside `unreviewed`. The numbers are in the
  coordinator's message; it copies them.

### D7. `review-reading-check.py` merges, and accepts the unbuilt shape

It becomes the merge the workflow script used to be: over a directory of
file readings and verdicts it produces the component reading — `findings`,
`changedNames`, `files`, `threads`, `unread` — validating each file against
the file-reading schema and naming a missing or invalid one as `unread`. A
reading with `unbuilt` set and every list empty is valid too, so the
coordinator is handed the component with `unbuilt: "<tail>"`, never as
`null`. `unreviewed` (no reading at all — the job failed), `unbuilt` (no
reader ran) and `unread` (a reader ran and returned nothing usable) stay
distinct, because they ask the author for different things.

## Risks / Trade-offs

- [A reader in its own process has no sibling's declares to judge against] →
  it never did: cross-file judgement is the coordinator's, from the
  readings' declares and references, and that is unchanged. The sibling
  NAMES stay in the prompt so a reference is recognisable.
- [The prompt cache expires between processes — a slow build step, a job
  that stalls] → the cost is the first-process price again, ~$0.10, never a
  wrong reading. Nothing depends on the cache for correctness.
- [Four processes at once hit API rate limits] → a refused process returns
  no reading and its path is `unread`, by name, and the width is one number
  in the workflow to lower. Not silent: the summary lists the path.
- [Blind reads keep finding things, so files take longer to go quiet than
  today's primed rounds did] → that is the finding the priming was hiding;
  the marker's `quiet` history measures it, and K stays at 1 so no read is
  spent beyond the first quiet one.
- [A file whose finding depends on another file that changed — a caller
  carried, a callee read] → the callee's reading declares the changed name;
  the coordinator's reach search now includes carried files and reads the
  consumer. That is the existing cross-check applied to a smaller exclusion.
- [A base-branch merge into the branch changes files the pull request never
  touched] → the decision runs over `gh pr diff --name-only` only; a file the
  merge brought in is not the pull request's and is not queued.
- [A rule file change on the BASE branch, merged in, changes what carried
  files would be judged against] → the invalidation checks `.claude/rules/**`
  between the recorded sha and head, whichever side brought the change.
- [The build script and CI drift — CI adds a step the prefilter lacks] → the
  script's test asserts each recipe against the tree, and `ci-green` remains
  the gate: the prefilter answers "worth reading", CI answers "mergeable".
- [The coverage marker grows with the pull request] → GitHub's comment bound
  is 65 536 characters; a hundred paths with a count each is a few
  kilobytes. The posting program truncates nothing: a marker that cannot be
  written is an error, never a silently shorter one that carries too much.
- [The landed dispatch commit (`Fixed in <sha>`) starts no review, and the
  next hand push reviews the delta including it] → unchanged from today, and
  the delta is exactly where a fix is re-read against its thread — by the
  verdict pass, which is the one context meant to hold the thread.
- [The runner's CLI is 2.1.247 and the facts were settled on 2.1.261] →
  fact 5 uses `--append-system-prompt`, `--json-schema` and the server-side
  cache, none of them new in that range; the first review after the merge
  is where the `usage` numbers in the job log confirm the cache reads, and a
  miss shows as cost, not as a wrong reading.

## Open Questions

None that change the specs or the tasks. The `node --test` recipe for a Node
runtime is a suite rather than a compile; if it proves slow the recipe can
become `node --check` over the runtime's files without a spec change. Whether
K should be 2 is answered by the marker's own history after a few pull
requests, and is a variable.
