## Why

The pull-request review reads a change per component "concurrently, bounded by
a pool the runtime sizes" — and on the runner it runs on, that pool is TWO.
The dynamic-workflow runtime caps concurrent `agent()` calls at
`min(16, max(2, CPUs − 2))`, computed once from `os.availableParallelism()`
with no override; `ubuntu-latest` has four vCPUs. On pull request #106 (eight
components) the readers started in pairs — two at 2:16, one at 4:17, one at
5:02, two at ~7:55, one at 9:29 — the run was stopped at exactly 600 s with two
still reading and the eighth never started, the coordinator never ran, and the
check failed. A healthy run takes 6–10 minutes for the same reason, and grows with
every component a change touches.

The other tenth of the time is spent on a model doing deterministic work: the
`queue` agent took 136 s and eleven denied commands to run `gh pr diff`, one
Python script and one GraphQL query — three commands a job step runs in
seconds. Across one run, readers accumulated 37 permission denials, each a
model round-trip that produced nothing.

A larger runner would lift the cap but is billed per minute. GitHub Actions
already offers what the review needs for free: twenty concurrent jobs on a
public repository. The loop the workflow script holds belongs in the Actions
matrix, where each reader gets its own four-core runner and its own process.

## What Changes

- **The review fans out as an Actions matrix, not as agents inside one
  session.** `claude-review.yml` becomes four jobs: `queue` (bash only — the
  changed paths, `review-queue.py`, the review threads, the change's delta
  specs; outputs the matrix), `read` (one matrix job per component, running
  the base branch's `component-reviewer` role through `claude -p` and
  uploading its reading as an artifact), `consolidate` (downloads every
  reading — a missing one is `unreviewed` — and runs the base branch's
  `review-coordinator` role, which posts as before) and the existing
  `reconcile`. The concurrency bound becomes the platform's job limit.
- **The queue is a script step.** No model builds it; the queue agent, its
  136 s and its denials are gone. The `read` matrix is `fromJSON` of the
  step's output; an empty queue skips `read` and the coordinator posts the
  empty summary.
- **A reading is a job.** It appears in the Actions UI by component name, is
  retried alone, fails alone, and its JSON is validated by a program against
  the reader's stated shape before upload — a reading that fails validation is
  the named `unreviewed` gap, as now.
- **The CLI is pinned and cached, through one composite action.**
  `.github/actions/claude-cli` installs a pinned `@anthropic-ai/claude-code`
  version from `actions/cache`, and restores the base branch's copies of the
  role files it is asked for. Every job that runs a model uses it. Today's
  unpinned `npm install -g` resolves "latest" per run — uncacheable, and the
  review's toolchain changing under it without a commit.
- **The workflow script is deleted.** `.claude/workflows/review-pr.js`
  encoded the loop; the loop is the YAML now, and two plans for one review
  drift. The local path is `gh workflow run claude-review.yml -f number=<pr>`
  (`workflow_dispatch` added), which runs the same jobs against the same
  roles; `dryRun` becomes an input that skips `consolidate`.
- **The reader's tool allowlist admits what it kept trying.** `git ls-files`,
  `git ls-tree`, `git cat-file`; and the role says what is NOT available
  (redirection, `/tmp`, `helm`, `go test`) so the model stops spending turns
  finding out.
- **The run-gate reads the same signal.** "The review actually ran" moves to
  `consolidate` and still requires the summary comment's count line; the
  execution artifact is per job.
- **Model concurrency is no longer a scenario.** The spec's "more components
  than the pool holds" scenario is replaced: every component is read at once,
  bounded only by the platform's job limit.

## Capabilities

### New Capabilities

_none_

### Modified Capabilities

- `automated-code-review`: the "reviewed per component, each in isolation"
  requirement — readings are started and collected by the CI workflow's matrix
  rather than a script inside one model session, run one per job with no
  runtime-sized pool, and the queue is built by a program; the "reviewer's
  definition is part of the guarded review" requirement — the guarded set is
  the two role files plus the composite action, and there is no saved workflow
  script; the local path is a workflow dispatch rather than a checkout command.
- `continuous-integration`: no requirement changes — the composite action and
  the pinned CLI are implementation. Listed to say it was considered.

## Impact

**Workflow and scripts**

- `.github/workflows/claude-review.yml` — rewritten as four jobs.
- `.github/actions/claude-cli/action.yml` — new composite action: pinned,
  cached CLI install plus base-branch restore of named role files.
- `.claude/workflows/review-pr.js` — deleted.
- `.claude/agents/component-reviewer.md` — header no longer names the script;
  tools widened; unavailable operations stated.
- `.claude/agents/review-coordinator.md` — header no longer names the script;
  readings arrive from files rather than a delegation message.
- `.github/tests/claude-review.test.sh` — rewritten for the matrix shape: the
  guard restores the role files in every model job, the queue step is model-free,
  the reader's allowlist, the pinned version, the summary gate on `consolidate`.
- `.github/scripts/review-reading-check.py` — new: validates a reading against
  the reader's stated shape (the validation the workflow runtime's schema did).

**Documents made untrue — reference half**

- `openspec/specs/automated-code-review/spec.md` — via the delta.
- `CONTRIBUTING.md` — "a saved workflow, `/review-pr` … the same command runs
  from a checkout, and `dryRun: true` returns the readings" describes the
  deleted script; becomes the workflow, dispatchable by hand.
- `.claude/rules/worktree-delivery.md` — "The review is a saved workflow with
  two roles — `.claude/workflows/review-pr.js` runs one `component-reviewer`
  per changed component … It runs from a checkout too: `/review-pr`".
- `.claude/rules/gotchas.md` — the "THE REVIEW'S FAN-OUT IS A WORKFLOW SCRIPT"
  and "a background `Workflow` never runs under the action" entries stay as
  history and gain the sentence that the fan-out is the Actions matrix now,
  and why: the pool is CPU-sized, with the measurement.
- `.claude/rules/retired-vocabulary.md` / `.github/retired-vocabulary.json` —
  `review-pr.js` and `/review-pr` as a current claim.

**Documents made untrue — adopter site**

- none. The site does not describe the repository's own review; checked
  `docs/` for `review-pr`, `claude-review`, `component-reviewer`,
  `review-coordinator` — no page names them.
