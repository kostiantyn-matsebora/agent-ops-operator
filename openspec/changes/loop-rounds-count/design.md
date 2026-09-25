## Context

The loop is four programs:

| Program | Holds |
|---|---|
| `review-dispatch.yml` | gate, collect, fix, land |
| `land-dispatch.py` | the endings and the round count |
| `autofix-guard.py` | the `docs-task` step and the `/opsx:archive` hook |
| `failed-checks.py` | the third reviewer's work items |

#244 bounded the fixing job at 30 minutes and taught `land` to read
`cancelled` as timed out, with the ending saying "no round was counted".

#248 then showed the cycle that leaves open. A timed-out round spends nothing
against the cap, and the guard's running-round refusal turns `ci-green` red,
which the gate reads as a reason for the next round.

## Goals / Non-Goals

**Goals:**

- A loop that times out every round stops at `max_rounds` like any other.
- `ci-green` is never red for a reason the loop itself made.
- A red made only of an unanswered dispute waits for a person and starts
  nothing.
- The archive hook keeps refusing while a round runs.

**Non-Goals:**

- Making a round finish faster, or bounding the work list a round takes. A
  round against 107 analysis issues will still time out. It will count.
- Changing what a person's reply re-runs (`dispute-answered.yml`).
- Any change to grants, carries or the label vocabulary.

## Decisions

- **The timed-out ending posts the round marker.** `land-dispatch.py` treats
  `--fix-timed-out` as a counted round: it posts the summary with the round
  marker, reports "rounds used N of M", and at the cap says so and names
  `conveyor:keep-going`. `--fix-failed` stays uncounted: no model ran, nothing
  was spent, and the two endings were kept distinct on purpose in #244.
  Alternative rejected: counting both. A crash before the model starts is
  the workflow's fault, and charging the person's budget for it hides that.
- **The guard grows a `--disputes-only` flag, and CI passes it.** The hook
  keeps the default, both questions. This is the same split the repository
  already made for review threads: a property the platform or a person
  resolves live is not a check's question. The running round is the loop's
  transient state, moved by `land`. Alternative rejected: dropping the guard
  step from `docs-task` entirely. The dispute question has no other check
  behind it, since a disputed analysis issue has no thread for conversation
  resolution to hold.
- **`failed-checks.py` reads the failed job's steps.** The jobs API lists a
  job's steps with their conclusions. A `docs-task` job whose only failed
  step is the guard step is dropped from the work list with a stated reason,
  so `collect` hands the fixer nothing and `land` ends the round as "nothing
  to do" with the dispute named. The step is matched by its name, read from
  `ci.yml` as the check names already are, never restated.
- **The cancelled-queued case needs no code.** Once the running-round
  question leaves CI, a run queued by a thread reply and cancelled by the
  concurrency group refuses nothing. It is written down in `gotchas.md` as
  the measurement that found the cycle, and covered by the guard test's
  disputes-only case.

## Risks / Trade-offs

- A timed-out round now spends the budget, so a loop against a large work
  list stops after five timeouts with nothing landed. That is the bound
  doing its job. The summary names the timeouts, and `conveyor:keep-going`
  grants more.
- With the running-round question gone from CI, `ci-green` can be green while
  a round is mid-push. Branch protection still requires the head to be
  up to date and every thread resolved, and the merge is a person's click.
- Reading job steps is one more `gh api` call in `collect`, under the
  `actions: read` it already holds.
