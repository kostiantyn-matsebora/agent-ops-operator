## Context

See proposal.md — Why. The review job runs `claude-code-action`, which is
`claude -p`; today its prompt makes the model build a queue, spawn one
subagent per component, wait, consolidate and post. Two run artifacts show
the model failing the "wait" both ways it can: ending its turn (#74), or
sleeping seven minutes (#77). What the review needs is a plan the model does
not hold: which readers run, on what, and what happens when they all return.

Dynamic workflows are exactly that (v2.1.154+, available under `claude -p`
and started without a prompt there). A saved workflow in `.claude/workflows/`
is a command; its script calls `agent()` per component through `pipeline()`,
concurrent up to the runtime's cap, each returning a schema-validated object;
the script then calls one more agent with all of them. Agent definitions in
`.claude/agents/` are the `agentType` those calls name, so each role is a
file with a system prompt and a tool allowlist.

Constraints kept from the current spec: per-component isolation, the reviewer
posts nothing, one summary, the thread-based dedup, the reach check, and the
guard that a pull request cannot rewrite the review that judges it.

## Goals / Non-Goals

**Goals:**

- The fan-out and the wait are code, not a model's turn.
- Two roles, each one file: `component-reviewer`, `review-coordinator`.
- The same review runs from a checkout: `/review-pr <n>`.
- The pool is bounded by the runtime, and the effective size is recorded.
- A reading that fails or returns prose is a named gap, never silence.

**Non-Goals:**

- Agent teams (interactive-only), a job matrix (per-runner setup cost), or
  any change to triage, dispatch, reconcile or the summary shapes.
- Changing how components are grouped (`review-queue.py` stays as it is; its
  granularity is a separate decision).

## Decisions

### 1. The orchestration is a saved workflow script, invoked by name

`.claude/workflows/review-pr.js`, `meta.name: review-pr`, taking `args`
`{number, base}`. The review job's prompt becomes one line: run `/review-pr`
with the pull request's number and base ref. Under `-p` the run starts with no
approval prompt. The script:

1. `agent()` — a `general-purpose` agent lists the changed paths (`gh pr diff
   --name-only`), runs `review-queue.py`, fetches every review thread once via
   the existing GraphQL query, and returns `{queue, threads}` by schema.
2. `pipeline(queue, entry => agent(reviewerPrompt(entry, threads, specs),
   {agentType: 'component-reviewer', label: entry.group, schema: FINDINGS}))`
   — concurrent, each in its own context, each returning the FINDINGS object
   the reviewer returns today (`component`, `findings[]`, `changedNames[]`,
   `threads[]{id, verdict}`) validated at the tool layer, so a prose return
   is retried by the runtime and then `null`.
3. `agent(coordinatorPrompt(results, threads), {agentType:
   'review-coordinator'})` — resolves reach, dedups, posts inline findings and
   the one summary, marks resolved threads, exactly as STEP 3–4 say today.
4. `return {reviewed, unreviewed, findings, summaryPosted}` and `log()` each
   phase's counts, which the run records.

*Alternative considered:* keep subagents, force foreground calls in one
message (#78). Rejected as the end state: it depends on the model obeying two
instructions it has ignored twice, and a violation is invisible until the
artifact is read.

### 2. Two agent definitions, in `.claude/agents/`

`component-reviewer.md` is the current inline definition, verbatim: its
tools (`Read`, `Grep`, `Glob`, `git diff/log/show`), its reading order, its
return shape. `review-coordinator.md` is the current prompt's STEP 3–4 with
the queue and spawn steps removed: tools `gh pr comment`, `gh api`, `git grep`,
`Read`, `mark-thread-resolved.sh`, the inline-comment MCP tool. Neither
carries a model; both inherit the session's.

*Why `.claude/agents/` and not `.github/review/`:* the workflow runtime
resolves `agentType` from the agent registry, which is `.claude/agents/`. A
definition elsewhere would have to be inlined back into `--agents`, which is
what this change removes.

### 3. The guard: restore the three files from the base ref before the run

The job already refuses to run when its own workflow file differs from the
base. The three new files get the same protection by a different mechanism —
`git checkout "origin/$BASE" -- .claude/workflows/review-pr.js
.claude/agents/component-reviewer.md .claude/agents/review-coordinator.md`
before the action step, and a `::notice` naming any of them the pull request
changed. The pull request's copies are then reviewed as ordinary files, and
land when the pull request merges, exactly as the spec now says.

*Alternative considered:* refuse to run, as for the workflow file. Rejected:
that leaves a change to the reviewer unreviewed, which the current
consolidator prompt already flags as THE FIRST FINDING; restoring the base
copy reviews the edit with the unedited reviewer, which is stricter.

### 4. The gate reads the run's outcome

"The review actually ran" keeps its execution-file check and its summary
check (the count line — both summary shapes carry it, #78). A workflow that
returned `unreviewed` components is not a failure — the summary names them —
but a workflow that did not return, or returned with `summaryPosted: false`,
fails the job.

### 5. The pool size is measured, not assumed

The runtime caps concurrent agents at 16, fewer on a CPU-limited runner. The
script `log()`s the queue length and the runtime reports how many ran at
once; the first run on `ubuntu-latest` records the effective pool in the
change's tasks. If it is 2, `runs-on` moves to a larger runner in a follow-up
— one line, and a cost decision the measurement makes rather than this design.

## Risks / Trade-offs

- **Workflows need a paid plan and a recent CLI** → the action installs the
  current CLI and authenticates with the same OAuth token the review uses
  today; the first run proves it, and the gate fails loudly if the command
  is unavailable.
- **`.claude/agents/` definitions load in every local session** → two more
  entries in the agent registry, invoked only by name; they carry no
  `background` or model override, so they change nothing unasked.
- **The keyword route (`ultracode`) does not work from `-p`** → not used; the
  saved command is invoked by name, which is the supported route.
- **Schema validation retries a prose return, then yields `null`** → the
  script records the component as `unreviewed` and the coordinator says so in
  the summary, which is the current behaviour stated more mechanically.
- **The coordinator still writes** — a single model turn posting comments →
  unchanged risk from today, now with its inputs complete by construction.

## Migration Plan

1. Land the three files and the workflow edit in one pull request. The
   action refuses to run the edited workflow file on that branch, as today.
2. The first review after merge is the test: the run's record, the summary,
   and the gate. Record the effective pool size in the tasks.
3. Rollback is reverting the workflow file; the agent files are inert
   without it.

## Open Questions

- Whether `ubuntu-latest`'s CPU count yields a pool larger than 2. Answered by
  the first run; the design does not depend on the answer.
