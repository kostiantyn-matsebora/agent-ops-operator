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

### D3 — `read` is one `claude -p` per matrix entry, which runs a FILE READER per changed file

Each job: checkout (full depth, for `base...HEAD`), the composite action
(D4) with `restore: .claude/agents/file-reviewer.md`, then one `claude -p`
whose only work is to run the saved workflow `review-component` (D8) and
return its merged reading:

```
claude -p "$PROMPT" --output-format json --json-schema "$READING" \
  --settings "$SETTINGS" --allowedTools "Workflow,Agent(file-reviewer)" > out.json
```

- The prompt is the component's delegation message — REPO, PR, BASE,
  COMPONENT, PATHS, THREADS ON THESE PATHS, DELTA SPECS — assembled by
  `review-prompt.py` from `review-input.json` for `matrix.entry.group`, and
  the instruction to run the workflow with it and return the result verbatim.
- `review-reading-check.py` validates the merged reading (D8's shape) and
  writes `readings/reading.json` — or exits non-zero, which fails the job by
  name. The job's failure does not fail the workflow (`continue-on-error:
  true`): `consolidate` reads an absent artifact as `null`, the named
  `unreviewed` gap.
- `--output-format json` rather than `stream-json`: one job, one result; the
  envelope's `structured_output` is the return. The stream form is kept for
  `consolidate`, whose transcript is the execution record the gate points at.

### D8 — `review-component.js`: a file reader per file, two at a time, merged by the script

The saved workflow holds the loop, for the reason on record (#74, #77): a
plan in a model turn is dropped with the turn. It runs `pipeline()` over the
component's changed files with `agentType: 'file-reviewer'`, schema-validated
returns, and merges.

- **A FILE READER's context is what that file needs and nothing else.** Its
  delegation message names: the file, the component and the OTHER changed
  files in it (names only — so a reference to a sibling is recognisable), the
  threads on that file, the delta specs, and THE RULE FILES TO READ for that
  path (D9). It reads its diff with `git diff -M <base>...HEAD -- <file>`,
  reads the file itself, reads the named rules, and returns.
- **The return is the reading plus the INTERFACE FACTS the cross-review
  needs:**

  ```
  {"path": "...", "findings": [...as today...],
   "declares":   ["<name added, removed or renamed — exact spelling, old and new>"],
   "references": ["<name used from outside this file — an import, a call, a field, an env var, a chart key, a path>"],
   "threads": [{"id": "...", "verdict": "fixed|standing|gone|detached"}]}
  ```

- **The script merges** into the component reading: `findings` concatenated,
  `changedNames` = union of `declares` (so the coordinator's existing reach is
  unchanged), a new `files[]` carrying each file's `declares` and
  `references`, `threads` concatenated, and `unread[]` naming any file whose
  reader returned nothing usable — a gap by name inside a component, the
  same rule as `unreviewed` between components.
- **Two at a time is the pool** on a four-core runner and it is not fought:
  the width of the review is the matrix. MEASURED (D10.1, run 33147062866):
  a file reader takes 44–221 s (median ~110 s) in a ~5–11 k-token context; a
  15-file component ran eight waves two-wide and the CLI stopped its workflow
  at 600 s — the ceiling is REAL, and the component went unreviewed. So the
  queue emits ONE MATRIX ENTRY PER TWO FILES (`review-input.py`, `CHUNK`):
  every reader in a job runs at once, no job nears the ceiling, and the
  width is the platform's. A chunk's readers are told the whole component's
  other file names as siblings; the coordinator's input merges a component's
  chunks into one reading, a chunk that produced none leaving its files in
  `unread` by name.
- **A session made to answer before its workflow finishes invents a
  reading.** Seen on the same run: with `--json-schema` the component
  session emitted an empty reading on its first turn, then the real one after
  the completion notification. `review-trace.py` accepts only a result that
  follows that notification, and the instruction says not to answer before
  it.
- **Dedup is the coordinator's**, by path + claim, as before.

### D9 — No job carries the rules; a reader is told which to read

Every `claude -p` in the review passes
`{"claudeMdExcludes":["**/.claude/rules/*.md"]}`. `CLAUDE.md` itself stays
(a few kilobytes: the index and the two named exceptions). What a context then
holds is its role file and its delegation message.

- **The routing table is a program**, `review-rules.py`, mapping a path to
  the rule files a reader of it must `Read`:

  | Path | Rules |
  |---|---|
  | `platform/manager/**` | `invariants.md`, `terminology.md`, `wiring.md`, `adapters.md`, `structure.md` |
  | `platform/manager/internal/ingest/**`, `signals/**` | the above plus `signal-rules.md` |
  | `runtimes/**`, `channels/**`, `gateways/**`, `platform/*` (other) | `invariants.md`, `terminology.md`, `adapters.md`, `structure.md` |
  | `chart/**` | `chart.md`, `wiring.md`, `invariants.md` |
  | `docs/**` | `documentation.md`, `terminology.md`, and `docs/CLAUDE.md` |
  | `platform/console/ui/src/theme/**`, `docs/assets/css/**` | `palette-and-mark.md` plus the row above |
  | `.claude/**`, `openspec/**` | `authoring.md`, `retired-vocabulary.md`, `terminology.md` |
  | `.github/**`, root files | `authoring.md`, `retired-vocabulary.md`, `structure.md` |

  Every path gets `retired-vocabulary.md`; nothing gets the six
  developer-session rules. The table is tested: every named file exists, and
  every rule file is reachable from some path — a rule no path routes to is
  one the review has silently stopped enforcing.
- **The coordinator reads no rules.** It dedups, greps and posts; its role
  file is its whole instruction.
- **The measurement that decides this stays in the record** (proposal — What
  Changes): 34 turns × ~76 k tokens on `consolidate`.

### D10 — Two measurements before the mechanism is trusted

Both run against this pull request, dispatched from the branch, and are
recorded on the tracking issue with their run links. No number is claimed in
any document before it is measured.

1. **The ceiling.** A component job whose workflow has more file readers than
   ten minutes at two-wide can finish. If the CLI stops the run at 600 s, the
   queue chunks components (D1's matrix, one entry per chunk) so no job's
   workflow exceeds what fits; if it does not, no chunking.
2. **The readings.** The per-file readings' findings on this pull request
   beside the per-component readings' findings already on it: what each found
   that the other did not. The cross-review from `declares`/`references` must
   catch the one cross-file finding the component reader made (the queue
   scripts unrestored) or the design is not done.

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

### D11 — The coordinator keeps its job, in a context that holds only its job

The coordinator's execution record on #111: 34 turns, 268 s of API time,
~76 k tokens re-sent per turn — of which the readings and threads were a few
thousand. The cost was the CONTEXT, not that a model dedups, judges and
posts; those are what a model is for, and a program doing them would be a
second place for the summary's shape and the thread rules to rot.

So the coordinator stays a role, and what changes is what it is handed:

- its role file, the readings and the threads, and no rule file (D9);
- the readings carry `files[].declares` / `files[].references`, and the role
  gains one instruction: a name declared removed or renamed in one file and
  referenced in another file's reading is a finding against that file, from
  the readings alone;
- for a consumer OUTSIDE the change (its `git grep`, as today) it reads that
  one file — a bounded read, not a whole-repository context.

Whether the remaining turns are slow is MEASURED after D9 (task 9.1), and
not pre-empted.

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
