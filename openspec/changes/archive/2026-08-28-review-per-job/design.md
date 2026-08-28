## Context

See proposal.md — Why. The measurement that decided this: the workflow
runtime's cap is `Math.min(16, Math.max(2, availableParallelism() - 2))`,
evaluated once at process start with no environment override (read from the
CLI binary, 2.1.247); `ubuntu-latest` is four vCPUs, so the pool is two. On
PR #106's execution artifact readers started at 136 s, 136 s, 257 s, 302 s,
474 s, 477 s, 569 s, and one never started; the run stopped at 600 s. The
queue agent alone spent 136 s and 11 denied commands; 37 denials in the run.

What stays: the two role files, `review-queue.py`, the thread GraphQL, the
coordinator's posting rules and summary shape, the `reconcile` job and its
resolver, the dispatch workflow, the summary-count run-gate, the fork/draft/
dependabot skips, and the concurrency group that cancels a superseded review.

What the platform gives: 20 concurrent jobs on a public repository at no
charge; `strategy.matrix` from `fromJSON(needs.X.outputs.Y)`; artifacts
between jobs; `workflow_dispatch` inputs.

## Goals / Non-Goals

**Goals:**

- Every reading starts at once; wall-clock is the slowest reading plus the
  coordinator, independent of component count.
- No model performs deterministic work (the queue, validation, the gate).
- One place for the CLI version and its install; one place for the
  base-branch restore.
- The same shape runs by hand, from GitHub, against any PR.

**Non-Goals:**

- Changing what a reader looks for, what the coordinator posts, or the
  triage/dispatch/reconcile contracts.
- A self-hosted or larger runner.
- Replacing the readers with the built-in `/code-review` skill — it returns
  findings and nothing else; the thread verdicts, reach and summary the rest
  of the pipeline consumes are the readers' and coordinator's.

## Decisions

### D1 — The loop is the Actions matrix, not a runtime pool

Four jobs: `queue` → `read` (matrix) → `consolidate` → `reconcile`.

- Why over a bigger runner: the cap is CPU-derived and there is no override,
  so lifting it costs a paid runner per minute for every review. Jobs are
  free and bounded at 20.
- Why over keeping `review-pr.js` and reading in the matrix: two plans for
  one review drift. Once the matrix holds the loop, the script is a second,
  differing description of it. Deleted; the local path dispatches the
  workflow (D6).
- Why a separate matrix rather than the build matrix: the build matrix is
  `components.sh` (image/module directories) and is path-filtered; the review
  queue is `review-queue.py` groups, which include `docs`, `chart`,
  `openspec`, `.github`, `.claude`, `root`. Coupled, half the diff would never
  be read and a reading would wait on a Go build.

### D2 — `queue` is bash and Python only

```
paths     = gh pr diff N --name-only
queue     = review-queue.py $paths            → [{group, kind, paths}]
threads   = gh api graphql … reviewThreads    → flattened, as today
specPaths = head is change/<name> ? git ls-files openspec/changes/<name>/specs : []
```

Outputs `queue` (the matrix `include` list, as JSON) and uploads
`review-input.json` (`{repo, number, base, paths, queue, threads, specPaths}`)
for the other jobs. An empty queue sets `read`'s `if:` false; `consolidate`
still runs and posts the empty summary. Needs no model and no secret beyond
`GITHUB_TOKEN`.

**The matrix is the CHANGED components only.** `review-queue.py` groups the
paths `gh pr diff --name-only` returns, so a component the diff never names
has no entry and no job — eight jobs on #106 because eight groups changed; a
one-file docs fix is one job. Nothing enumerates the tree.

Matrix size is bounded by GitHub at 256; `review-queue.py` produces at most
one entry per component plus a handful of directory groups, far below it.

### D3 — `read` is one `claude -p` per matrix entry, returning a file

Each job: checkout (full depth, for `base...HEAD`), the composite action
(D4) with `restore: .claude/agents/component-reviewer.md`, then:

```
claude -p "$PROMPT" --agent component-reviewer --output-format json \
  --settings "$EXCLUDES" --allowedTools "<the reader's tools>" > out.json
```

- The prompt is the same delegation message the script built: REPO, PR, BASE,
  COMPONENT, PATHS, THREADS ON THESE PATHS, DELTA SPECS — assembled by a
  Python step from `review-input.json` for `matrix.group`.
- The reader is asked to return ONLY the JSON; `review-reading-check.py`
  extracts the `result` from the CLI's JSON envelope, parses the first `{…}`
  in it, validates against the reader's shape (the same four required keys
  and enums the script's `FINDINGS` schema held), and writes
  `reading-<group>.json` — or exits non-zero, which fails the job by name.
  The job's failure does not fail the workflow (`continue-on-error: true`):
  `consolidate` reads an absent artifact as `null`, the named `unreviewed`
  gap, exactly the script's semantics.
- `--agent` reapplies the role's `tools:` as an availability intersection.
  That is the behaviour `wiring.md` bans for RUNTIME pods (it defeats
  `overwrite`); here it is wanted — it is the allowlist.
- Widened tools: `Bash(git ls-files:*)`, `Bash(git ls-tree:*)`,
  `Bash(git cat-file:*)`. The role states what is unavailable — redirection,
  paths outside the checkout, `helm`, `go`, `python3` — so a reader stops
  probing.
- `--output-format json` rather than `stream-json`: one job, one result; the
  envelope's `result` is the return. The stream form is kept for
  `consolidate`, whose transcript is the execution record the gate points at.

### D4 — `.github/actions/claude-cli`: pinned, cached, and the restore

Inputs: `version` (default from the action, the ONE place it is written),
`restore` (newline list of paths to take from the base branch), `base-ref`.

1. `actions/cache` on `~/.npm-global/lib/node_modules/@anthropic-ai/claude-code`
   and `~/.npm-global/bin/claude`, key `claude-cli-${version}-${runner.os}`.
   `npm config set prefix ~/.npm-global` first, so the path is deterministic
   and user-owned. Miss → `npm install -g @anthropic-ai/claude-code@${version}`.
2. `claude --version` must print the pinned version — a cache that restored
   the wrong thing fails here rather than reviewing with it.
3. For each `restore` path present on `base-ref`: `git checkout <base> -- <p>`;
   emit `restored=` output listing those that differed, which the calling job
   turns into the existing `::notice::`.

Why a composite over a reusable workflow: it runs inside the matrix job's
checkout, on the job's runner, and adds no job of its own. Why the restore
lives here: three jobs need it and each would otherwise carry the loop; a
composite that installs the model's tooling and pins what it reads is one
guarded surface, which is what the spec now names.

The action file itself is restored first, by the calling job, with the same
one-liner — a composite is a file in the checkout, so a pull request could
edit it; the workflow file's own guard covers the caller, and the caller
restores the action before `uses:` resolves it. (`uses: ./.github/actions/…`
resolves at job start from the checkout as it is after `actions/checkout`,
so the restore must be a step before it — verified by reading the docs; the
test asserts the order.)

### D5 — `consolidate` runs the coordinator over files

`needs: [queue, read]`, `if: always() && needs.queue.result == 'success'`.
Downloads `review-input.json` and every `reading-*` artifact into `readings/`;
a Python step assembles the coordinator's delegation message exactly as the
script did — READINGS as the queue mapped to `{group, reading|null}` — and
`claude -p` runs `review-coordinator` with its tools, `stream-json` to the
execution artifact. Then the existing "review actually ran" gate (summary
count line present), the `.resolve-threads` touch and upload. `reconcile` is
unchanged and `needs: consolidate`.

### D6 — By hand: `workflow_dispatch`

Inputs `number` (required) and `dry_run` (boolean, default false). With
`dry_run`, `consolidate` is skipped and the readings are the run's artifacts —
the script's `dryRun` semantics. The trigger conditions (`draft`, `fork`,
`dependabot`) read `github.event.pull_request` on the PR event and are
bypassed on dispatch, which is a person asking; `queue` resolves the PR from
the input in that case. `/review-pr` is retired vocabulary; the replacement
is `gh workflow run claude-review.yml -f number=<pr>`.

### D7 — The tests follow the shape

`claude-review.test.sh` is rewritten around the new file: the three model
jobs use the composite; each restores the action first and names its role in
`restore`; `queue` has no step running `claude`; the reader's allowlist holds
the git read commands and nothing that writes; the matrix reads
`needs.queue.outputs.queue`; `read` has `continue-on-error`; the gate lives
in `consolidate`; the version appears once, in the action; `workflow_dispatch`
carries `number`. `review-reading-check.py` gets its own test: a valid
reading passes, prose fails, a missing key fails, a bad verdict enum fails,
a JSON envelope with the JSON inside prose is extracted.

## Risks / Trade-offs

- [Eight runners × ~40 s of checkout and install] → the cached install brings
  the install to seconds; checkout with `fetch-depth: 0` is the real fixed
  cost. All in parallel; wall-clock unchanged.
- [A reader job that fails does not fail the workflow] → it is the named
  `unreviewed` row in the summary, as before; the summary gate still fails a
  review that posted nothing. The job is red by name in the UI.
- [`--agent` semantics differ from the workflow runtime's `agentType`] → the
  role's `tools:` is the allowlist either way; the test pins the flags.
- [`fromJSON` matrix on an empty list errors] → `read` is `if:
  needs.queue.outputs.count != '0'`; `consolidate` handles zero readings.
- [The composite is in the checkout and a PR edits it] → restored from base
  by the caller before `uses:` resolves; the notice names it; the test asserts
  the order.
- [The dispatch bypasses the fork skip] → a dispatch is by a person with
  write access, on this repository; a fork PR dispatched by hand has a
  read-only token and fails at the first `gh` write, visibly.
- [`cancel-in-progress` now cancels a matrix] → correct: a superseded review
  should stop all of its readings, and the group is keyed on the PR.

## Open Questions

- ~~Whether `--json-schema` on `claude -p` (if present in the pinned version)
  can replace `review-reading-check.py`'s extraction.~~ SETTLED during apply:
  CLI 2.1.247 lists `--json-schema <schema>` in `--help`, the `read` job
  passes the reader's shape through it, and every reading on #111 arrived in
  the envelope's `structured_output`. `review-reading-check.py` still
  validates — the schema is what the CLI enforces, the check is what the job
  trusts — and still accepts a text `result` so a CLI that drops the field
  does not fail every review.
