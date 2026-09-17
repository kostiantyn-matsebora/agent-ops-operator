## Context

See proposal.md for the four measured defects. What shapes the fix:

- **The line is three workflows and four scripts**, all under `.github/`, all
  tested by `.github/tests/run.sh` with a stubbed `gh`. Nothing here touches
  the manager, the chart or a cluster.
- **A grant is a label a person places, and a program may only carry or
  consume it.** Every fix below keeps that: no new label grants anything.
- **A `workflow_dispatch` run started by a workflow has `github-actions[bot]`
  as its actor.** The gate already accepts that actor after re-reading the
  standing instruction. The Claude action does not, by default.
- **A `pull_request` run's check runs are the only ones the merge box reads**
  (`gotchas.md`, #131). Any re-evaluation of a check must re-run a job of the
  original `ci` run, never start a run under another event.
- **Labelling with the workflow token fires no event.** Every station a
  workflow starts must be started explicitly, which is what the `open` job's
  re-dispatch already does for `fix`.

## Goals / Non-Goals

**Goals:**

- One `conveyor:run` takes an opsx-lane issue from implement to archived, and
  a plain-lane issue to merged, with a person acting only at the merges and
  on disputes.
- Every station's outcome is visible on the issue or the pull request. Nothing
  ends in a log alone.
- A person can read the station and the loop state from labels, and the
  vocabulary lives in one file.

**Non-Goals:**

- Merging without a person. Nothing here merges.
- A second tracking surface beyond labels. The alternatives are listed below
  for the maintainer to choose from, and none is built by this change.
- Repairing #220 itself. It needs a hand re-run today and will be driven by
  the merged line afterwards.

## Decisions

### 1. The fixing job names the one bot that may start it

`allowed_bots: github-actions` on the `claude-code-action` step, and nothing
wider.

- **Why not `*`:** the action's own warning. On a public repository `*` lets
  an external App start the action with a prompt it controls.
- **Why it is safe:** the `gate` job runs first and refuses a bot dispatch
  unless the pull request's `Refs #<n>` issue still carries a writer's
  `conveyor:run`. The action's check is redundant with that gate and is kept
  as the second wall, narrowed to the platform's own bot.
- **Why not move the gate into the action:** the gate posts a readable refusal
  on the pull request. The action's refusal is a job failure.

### 2. `land` runs on a failed `fix`, and says so

`land`'s `if:` accepts `needs.fix.result == 'failure'`. A step then
substitutes an empty patch and report, exactly as the skipped case does, and
passes `--fix-failed` with the run's URL. `land-dispatch.py` posts one summary,
ending `fixing step failed`, and sets the loop label to `stalled`.

- **Distinct from `no report`.** `no report` means the model ran and wrote
  nothing. `fixing step failed` means no model ran. Reading one as the other
  tells a person a machine looked when it did not, the same failure
  `worktree-delivery.md` records for silence.
- **No round is counted**, because nothing landed. The label stays, and the
  summary names what restarts a round: a push, or `conveyor:keep-going`.

### 3. `review-clean` reads the threads live, and a thread event re-runs it

Two halves:

1. **Where the gate lives.** `claude-review.yml`'s `reconcile` job drops the
   "leaves nothing open" step. The review run's conclusion then means "the
   review ran and posted". `ci.yml`'s `review-clean` job runs
   `review-is-green.py` (the review ran) and then `review-not-clean.py` (no
   review-authored thread is open), both against the live state.
2. **When it re-runs.** A new workflow `review-thread.yml` on
   `pull_request_review_thread` (`resolved`, `unresolved`) runs
   `rerun-review-clean.py`, which finds the head's latest `pull_request` run
   of `ci` and re-runs its `review-clean` job through the jobs API. Re-running
   a job re-runs its dependents, so `ci-green` re-evaluates in the SAME run,
   and the merge box sees it.

- **Why not re-run all of `ci`:** cost, and nothing else changed.
- **Why not a job on the thread event itself:** its check runs never reach the
  merge box (#131).
- **A `ci` still in progress needs nothing**, since `review-clean` reads live
  when it runs. A missing run, or a fork's read-only token, is a notice.
- **The carry job fires again on the re-run's completion** and is idempotent:
  the label is re-asserted and the marker comment stops a second dispatch.
- **The dispute marker for checks stays as it is.** A head still red after a
  resolution starts a round that disputes the same check again, at the cost
  of one model run and no round counted. Bounded by events, not by rounds.

### 4. The archive station is a remote session, started by the carried label

- **`remote-implement.yml`'s `fire` job also fires on `conveyor:archive`.**
  `remote-implement.py` accepts the archive label, and accepts a sender of
  `github-actions[bot]` only after re-checking that `conveyor:run` stands on
  the issue and its placer can push, the same check `carry-grant.py` makes.
  Any other bot, or a person without write access, is refused as today.
- **The fire record is per station.** The implement marker stays
  `<!-- remote-implement:fired -->`, unchanged for every issue that carries it.
  The archive station records `<!-- remote-implement:fired:archive -->`.
- **One routine, two instruction files.** The saved prompt still points at
  `implement-issue.md`. Its first section reads the issue's labels: an
  `conveyor:archive` sends the session to `archive-change.md`. The archive
  file is the committed process: find the change by its `.github-issue`,
  confirm its pull request merged, take `change/<name>` (recreated from
  master where the merge deleted it), `openspec validate`, `openspec archive`,
  `opsx-issue.sh phase archived`, commit, push, open the pull request with
  `Closes #<n>` and no label, stop.
- **The archive pull request is driven by the same loop.** `carry-from-pr.sh`
  reads `Closes #<n>` where `Refs #<n>` is absent, so `open` carries
  `conveyor:fix` onto it. The `archive` job reads a merged pull request that
  says `Closes` as the line's end and sets the station to `done`.
- **The archive gate holds.** The session's `openspec archive` passes through
  `require-docs-task.sh` like anyone's: ticked tasks, and the fixing loop
  closed. The loop is closed because the pull request is merged, and the
  guard fails open on a closed one.

### 5. Two state vocabularies, one script, moved at the transitions

`review-triage.json` gains `station_labels` (issue: `station:implement`,
`station:fix`, `station:merge`, `station:archive`, `station:done`) and
`loop_labels` (pull request: `loop:running`, `loop:stalled`, `loop:capped`,
`loop:mergeable`). `conveyor-state.py --target <n> --station <x> | --loop <x>`
adds one and removes its siblings, idempotently, and never fails a job.

| Transition | Set by | Sets |
|---|---|---|
| implement session fired | `remote-implement.py` | issue `station:implement` |
| `conveyor:fix` carried onto the pull request | `carry-grant.py` | issue `station:fix` |
| a round starts | `gate` | pull request `loop:running` |
| a round ends with disputes only, no report, or a failed fixer | `land` | pull request `loop:stalled` |
| the round cap is reached | `land` | pull request `loop:capped` |
| `ci` succeeds on a labelled pull request | the `open` job | pull request `loop:mergeable`, issue `station:merge` |
| a merge carries `conveyor:archive` | `carry-grant.py` | issue `station:archive` |
| the archive pull request merges, or a plain-lane pull request merges | the `archive` job | issue `station:done` |

- **Distinct prefixes from the grants**, so nobody reads `station:fix` as
  consent. Their descriptions say so.
- **The `opsx:` phase labels stay.** They say where the CHANGE is in its own
  lifecycle. The station labels say where the LINE is, on both lanes.
- **Created by hand once**, as the `conveyor:` labels were. The task list
  carries the commands.

### 6. Second tracking surfaces, for the maintainer to choose from

None is built here. Each is one further change.

| Surface | Gives | Costs |
|---|---|---|
| one status comment on the issue, edited in place at every transition | station, pull request, round, last event, run link, in one table | a marker comment per issue, and every workflow editing it |
| a `conveyor` check run on the pull request head | the loop state in the merge box | `checks: write` in a model-free job |
| a Projects board with a station field driven by the labels | a board across every change | a project, and a token with project scope |
| a `conveyor-status.sh <issue>` command | the whole line from a terminal, from labels and markers | nothing on the platform |

The first is the one to build next if labels prove too little.

## Risks / Trade-offs

- [The bot allowlist widens who may start the model] → bounded to the
  platform's own bot, behind a gate that re-reads the grant first.
- [A thread event storm re-runs `review-clean` many times] → one job per
  event, concurrency keyed by the pull request, and a run in progress is left
  alone.
- [The archive session's branch was deleted by the merge] → the routine
  recreates it from master. The archive commit needs nothing from the old
  branch.
- [State labels drift when a workflow fails mid-transition] → they grant
  nothing and every transition re-asserts its own value, so the next
  transition corrects them.
- [A person mistakes `station:fix` for a grant] → the description on each
  label says it is state, and the vocabulary file's comment says so.

## Migration Plan

1. Merge. Every workflow takes effect from master at once.
2. Create the nine state labels by hand, once.
3. #220: re-run its review's `reconcile` job and CI's failed jobs once by
   hand. From then on the merged line drives it.
