# The fixing loop counts every round and never feeds on its own refusal

## Why

**The fixing loop fed on its own refusal, and its bound never applied.**
Under a standing `conveyor:run`, the loop on #248 could neither end nor let
the pull request merge. The cycle, measured on 2026-09-25:

1. A review completion starts a round.
2. The round runs to the fixing job's time limit against 100+ open analysis
   issues and is landed as "no round counted".
3. While it runs, the `docs-task` check refuses on "a round is still
   running", so `ci-green` is red.
4. A red `ci-green` starts the next round.

The loop fed on its own refusal, and the bound never applied because a
timed-out round counts for nothing. Getting the pull request merged took
removing the grant from the tracking issue by hand, which also cost the
archive station its actor.

## What Changes

- **A timed-out round counts.** A fixing job that hits its time limit ends the
  round as "timed out", and that round counts toward `max_rounds` exactly as a
  landed one does. Five timeouts in a row reach the cap and stop, with the
  summary naming them.
- **A CI check never carries the running-round question.** `docs-task` keeps
  asking whether a dispute the loop posted has no answer from a person, and
  stops asking whether a round is running. Whether a round is running is the
  loop's own transient state, and a check that reports it red starts the
  next round from its own red. The `/opsx:archive` hook keeps both questions.
- **The loop's own bookkeeping never starts a round.** The failed-checks
  reader excludes a `docs-task` failure whose failing step is the loop guard,
  so a red `ci-green` made only of an unanswered dispute waits for the person
  it is waiting for instead of dispatching a fixer that can do nothing about it.
- **A queued dispatch run that the concurrency group cancels never refused a
  check** — it follows from the second item, and the case is written down:
  six thread replies right after a push queued six runs and failed `docs-task`
  in its first ten seconds, three pushes in a row.
- **The gate accepts the grant the carry accepted.** #238 made
  `conveyor:archive` on the tracking issue its own grant for the archive pull
  request's fix station, and `carry-grant.py` places `conveyor:fix` on it.
  The gate in `review-dispatch.yml` still re-checks a bot-started round
  against `conveyor:run` alone, so it refused the round on #254 that the
  carry had just authorised. The gate reads the same rule as the carry.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `conveyor-lifecycle`: the bound counts every round that ran, timed out
  included, and the line's own state is never a check's verdict — a running
  round is not a failed check, and a failed check that is only the loop's own
  guard starts no round. Every program that re-checks a carried grant reads
  the same rule for what a standing grant is.

## Impact

- `.github/scripts/land-dispatch.py`: the `--fix-timed-out` ending posts the
  round marker and reports the rounds used. `--fix-failed` stays uncounted,
  since no model ran at all, which is distinct from a model that ran out of
  time.
- `.github/scripts/autofix-guard.py`: a `--disputes-only` mode for CI. The
  hook keeps the default.
- `.github/workflows/ci.yml` (`docs-task`): calls the guard in disputes-only
  mode.
- `.github/scripts/failed-checks.py`: reads the failed job's steps and
  excludes `docs-task` failed by the guard step.
- `.github/workflows/review-dispatch.yml` (`gate`): a bot-started round is
  accepted when the named issue carries `conveyor:run`, or `conveyor:archive`
  where the pull request is the archive one, in both places it re-checks.
- `.github/tests/land-dispatch.test.sh`, `autofix-guard.test.sh`,
  `failed-checks.test.sh`: one case each.
- `.claude/rules/worktree-delivery.md`, `.claude/rules/gotchas.md`,
  `docs/CHANGELOG.md`: the rule and the measurement.
