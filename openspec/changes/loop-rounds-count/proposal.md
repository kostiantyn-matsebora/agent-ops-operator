# The conveyor is one state machine, and the loop cannot cycle on its own refusal

## Why

**The conveyor broke on every use, and each fix was another copy of a rule.**
The label-driven line (implement, fix, archive) is decided in five workflows
and eleven programs.

Each holds its own idea of what a grant is, when a round
starts and what state comes next. Drawing every label, station, loop and
transition from the published diagram found ten places where two of them
disagree:

1. The gate re-checks a bot start against `conveyor:run` alone, while the carry
   accepts `conveyor:archive` for the archive pull request. Every round on #254
   was refused.
2. A round that hit its time limit counted for nothing, so `max_rounds` never
   bound it (#248).
3. `docs-task` refuses while a round runs, and a red `ci-green` starts the
   next round. The loop fed on its own refusal (#248).
4. Every pull request comment starts a dispatch run, and a queued run failed
   `docs-task` in its first ten seconds (#248).
5. The archive carry fires on any `change/*` merge, a proposal merge included
   (#255).
6. The label fire and the archive carry can each start a session for one issue
   at once (#255).
7. `conveyor:run` always fires the implement station, and its fire record is
   permanent, so a finished change is never archived by it (#255).
8. A round that ended clean with a thread open left `loop:running`, which the
   corrector skips.
9. `open` dispatches a round while the review's completion starts another for
   the same head.
10. Nothing enforced the cap before a round started, so the commit a capped
    round pushed started the next one.

**Fixing ten copies one at a time is what kept failing.** The rule has to
exist once, and every program has to ask it.

## What Changes

- **One state machine, `.github/scripts/conveyor.py`.** Pure functions, no I/O.
  It holds the station table, the loop table and every decision the workflows
  make: the standing grant, the fire, the two carries, the gate, the guard, the
  ending of a round, whether a failed check is work, the recovery of an
  orphaned `loop:running` and the refresh of a stale label.
- **The programs become adapters.** Each gathers facts, asks the machine, and
  does what it answers. `carry.py` replaces `carry-grant.py` and
  `carry-from-pr.sh`. `dispatch-gate.py` replaces the gate's shell.
  `conveyor-state.py` takes events, never values, and is the only writer of a
  state label.
- **Stations and loops are covered, not only the grant.** Every state and every
  event of both tables, every combination of each decision's facts, and each
  measured failure as a named case.
- **A round that ran a model counts, whatever ended it.** Timed out, disputed,
  no report and stale patch count. A clean round and a fixing job that never
  started do not.
- **The cap is enforced before a round starts.** A round begins while rounds
  used are below the ceiling or `conveyor:keep-going` stands.
- **A carried grant is re-checked on every start**, completions included.
  Removing `conveyor:run` from the issue stops a loop already running.
- **No required check reports whether a round runs.** `docs-task` asks the
  dispute question alone. The archive hook keeps both. A red made only of the
  loop's own guard starts no round.
- **The archive carry needs a finished change**, and a fire follows the
  change's stage and starts one session per line at a time.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `conveyor-lifecycle`: the line's decisions are one machine that every program
  asks. The bound counts every round that ran and is enforced before the round.
  A running round is not a check's verdict. Every re-check of a carried grant
  reads one rule, and the archive station needs a finished change.

## Impact

- `.github/scripts/conveyor.py` and `conveyor_io.py`: the machine and the shared
  fact gathering. New.
- `.github/scripts/carry.py`, `dispatch-gate.py`: new adapters.
  `carry-grant.py` and `carry-from-pr.sh` are deleted with their tests.
- `.github/scripts/conveyor-state.py`, `remote-implement.py`,
  `land-dispatch.py`, `autofix-guard.py`, `failed-checks.py`,
  `refresh-loop-state.py`, `recover-loop-state.py`: rewritten over the machine.
- `.github/workflows/review-dispatch.yml`, `remote-implement.yml`, `ci.yml`: the
  gate step, the `open` and `archive` jobs and the `docs-task` guard call.
- `.github/tests/`: `conveyor.test.py` and one suite per adapter.
- `.claude/rules/worktree-delivery.md`, `remote-session.md`, `gotchas.md`,
  `.github/routines/implement-issue.md`, `docs/CHANGELOG.md` and the published
  lifecycle diagram.
