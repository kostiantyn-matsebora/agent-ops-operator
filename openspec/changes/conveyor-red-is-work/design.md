## Context

See proposal.md for the #259 measurement. What shapes the fix:

- **The line is one state machine, `conveyor.py`, and the workflows are its
  adapters.** Every rule below lands as a table entry or a decision function
  there, tested without I/O, and a program only gathers facts and asks.
- **The dispute guard has two halves.** `autofix-guard.py --purpose ci` is a
  `docs-task` step. `--purpose archive` is the `PreToolUse` hook on
  `openspec archive`. Only the first is a check.
- **A thread's state is not a check's question.** Branch protection's
  required conversation resolution holds the merge on an open thread, live.
  #220 froze a check on that question once, and the rule was written then.
- **A thread resolution is not an Actions trigger.** `gotchas.md` records the
  refused `pull_request_review_thread` workflow. Comments are triggers.
- **A round's triggers today.** A review completion, a CI run concluding
  `failure`, a grant label, `/fix-accepted`, `workflow_dispatch`. Nothing
  starts on a CI `success`, and nothing starts on its own after a round that
  landed nothing.
- **A `workflow_dispatch` started by a workflow has `github-actions[bot]` as
  actor**, and the gate already re-checks the standing grant for that actor.
  `land` may therefore start the next round itself the way `open` starts the
  first.

## Goals / Non-Goals

**Goals:**

- Every red on an approved pull request's head is either fixed by a round or
  disputed by the fixing step, and nothing else stops the loop below the
  bound.
- A person can read from one label whether the loop is working, waiting on
  them, or stopped on its own.
- A dispute answered by a reply or by a resolution reaches the loop without a
  push or a hand re-run.
- No round claims anything about checks that have not spoken.

**Non-Goals:**

- Merging without a person. Nothing here merges.
- Answering a dispute for the person. A dispute the fixing step made still
  waits.
- Changing what the review finds or how the fixer fixes. This change is the
  loop around them.

## Decisions

### 1. The guard's CI half is deleted, not narrowed

The `docs-task` step "No dispute the fixing loop posted waits unanswered" is
removed from `ci.yml`. `autofix-guard.py` keeps `--purpose archive` and the
hook keeps calling it. `conveyor.guard` loses the `ci` purpose.

- **Alternative considered: re-read live in `check_is_work`.** Keep the
  exemption but ask the threads whether the dispute still stands, and re-run
  `docs-task` when it does not. Rejected: it keeps a check carrying the
  loop's conversation, which is the class of defect #220 and #248 were, and
  it needs a re-run path for a fact the check should never have held.
- **What holds the merge instead.** The disputed thread, open, through
  branch protection. A disputed analysis issue or check has no thread, and
  its red or its Sonar gate is what holds the merge if anything does. The
  summary and `loop:waiting` are the notification.
- **`dispute-answered.yml` and `rerun-ci-job.py` go with it.** Their only
  purpose was re-running the deleted step. Decision 6 gives the comment a
  new meaning.

### 2. `check_is_work` has two answers, `skip` and `work`

The `waiting` action and the `guard_step` parameter are deleted, and with
them `failed-checks.py`'s `guard_step_name` and `waiting` output. A failed
required check is work. `ci-green` and non-required checks are skipped as
before.

### 3. A claimed fix the patch does not evidence is unaddressed

`land-dispatch.py`'s `evidence` failure moves an item from `disputed` to
`unaddressed`, reason kept ("reported fixed, but the patch does not touch
…"). No thread reply, no dispute marker. The summary's Unaddressed section
lists it with the reason.

- **Why not keep it a dispute with a better reason.** A dispute is a
  statement the fixing step made. The landing step inferring one puts words
  in the fixer's mouth and a question on a person's desk.
- **Why not silently retry.** The summary must say what happened to every
  item, so a reader can see a fixer failing the same item three rounds in a
  row and act before the cap.

### 4. `loop:waiting` joins the vocabulary

`review-triage.json` gains `loop_labels.waiting`. `conveyor.py`:

| Change | Detail |
|---|---|
| `LOOP_STATES` | `none, running, waiting, stalled, capped, mergeable` |
| `LOOP_EVENTS` | `end:waiting` added |
| transitions | `running → waiting` on `end:waiting`. From `waiting`: `round:started → running`, `ci:green_clean → mergeable`, `thread:opened` SKIP |
| `ending()` | `clean` with a thread open or disputes remaining → `end:waiting`. `disputed` (every remaining item disputed, nothing fixed) → `end:waiting`. `stalled` keeps `no report`, `failed`, `timed out`, `stale patch`, `could not start` |
| `refresh()` | `mergeable` with a thread opened → `waiting`, not `stalled` |
| `recover()` | a superseded `running` with a thread open → `waiting` |

`stalled` then means exactly one thing: the machine stopped without a
decision, and a person reads the summary to learn why.

### 5. A round that lands nothing continues itself

`land` gains `actions: write`, and when the ending is `unaddressed` (nothing
fixed, at least one item eligible) or `stale patch`, and the round count is
below the cap, it dispatches `review-dispatch.yml` with `mode=all` after
posting the summary. The round counts, as every round that ran a model does.

- **Why `land` and not a trigger.** Nothing was pushed, so no review and no
  CI run concludes. The `open` job already dispatches this way for the first
  round.
- **Why the summary is still posted.** Every round is one summary. A reader
  following the pull request sees round 3 say "fixed nothing, retrying
  item X", and can remove the grant.
- **The bound is the only brake.** A fixer that cannot land the same item
  spends the five rounds and stops `capped`. That is the ceiling's job.

### 6. A person's comment on a waiting pull request starts a round

The gate's `issue_comment` and `pull_request_review_comment` branches accept
a non-bot comment on a pull request that carries `conveyor:fix` and
`loop:waiting` as a `mode=all` start, re-checking the grant as every start
does. `/fix-accepted` keeps its meaning on every pull request.

`accepted-findings.py`'s `classify_all` treats a disputed thread as awaiting
only while no person commented after the marker. The `unanswered_after_marker`
rule moves from `autofix-guard.py` into `conveyor.py`, so both programs read
one copy.

- **Why any comment and not a phrase.** The person is answering a dispute in
  its thread, in their own words. Asking them to also type a token is the
  hand step the conveyor exists to remove.
- **Why only in `waiting`.** A comment on a running or capped loop starts
  nothing, so a conversation on a pull request does not spend model runs.

### 7. A scheduled sweep resumes a waiting loop nobody commented on

A new `conveyor-sweep.yml`, on `schedule` every fifteen minutes and on
`workflow_dispatch`, runs one model-free job: list open pull requests
carrying `conveyor:fix` and `loop:waiting`, and for each with no unanswered
dispute, dispatch `review-dispatch.yml` with `mode=all`. Everything else is
left alone.

- **Why a sweep.** A thread resolution is the dismissal the triage table
  advertises, and it fires no event. Nothing else can see it.
- **Cost.** A few API reads per waiting pull request, four times an hour,
  and a round only when something changed.
- **The gate still decides.** The sweep dispatches. The gate re-reads the
  grant and the head's runs as for every start.

### 8. A round starts only on a concluded head

The gate gains one decision, `conveyor.round_may_start(ci_concluded,
review_concluded)`. Before any `mode=all` start the gate reads the head's
`ci-green` check run and the head's review run, and defers with a notice when
either is in progress.

The `workflow_run` branch accepts a CI conclusion of `success` too, so the
completion that arrives second starts the round.

`failed-checks.py` reports `consulted` only when every required check run has
`status: completed`.

- **Why both runs.** Two starts on one head ran in sequence today and both
  counted. One start per head, on the whole picture, spends one round.
- **What a CI `success` start does.** With nothing on the work list the
  round ends `clean` and marks mergeable, which is what `open` does today.
  With review threads open it fixes them.

## Risks / Trade-offs

- **A disputed flaky check leaves the pull request red in `loop:waiting`.**
  → The summary names the check and the log's reason. A person re-runs the
  job or removes the grant. This is the honest wait.
- **A disputed analysis issue or check no longer blocks the merge on its
  own.** → It never did through anything but the deleted check. The Sonar
  gate and the check's own red still hold where they apply, and the label
  and summary say a person is owed a ruling.
- **Self-continuing rounds spend the bound on an item the fixer keeps
  failing.** → That is what the bound is for, and every round's summary
  names the item. Removing the grant stops it at once.
- **Any comment on a waiting pull request starts a model run.** → Only under
  `conveyor:fix` and only in `waiting`, and the round counts toward the
  bound.
- **The sweep runs on a schedule the repository pays for.** → One model-free
  job, most runs ending in a listing with nothing to do.
- **Deferring starts to the concluded head delays a round by a CI run.** →
  Ten minutes, against a round that otherwise reports on jobs that have not
  spoken and then runs again when they fail.

## Migration Plan

Repository tooling only. The change merges as one pull request. Its own
review runs under the old workflow copies, so the first pull request to feel
the new rules is the one after it.

`loop:waiting` is created as a label on the repository in the same change,
since `conveyor-state.py` applies labels and does not create them.
