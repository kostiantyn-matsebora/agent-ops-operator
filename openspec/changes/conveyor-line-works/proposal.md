## Why

The conveyor line does not run. `conveyor:run` on #51 implemented the change
and opened #220, a workflow carried `conveyor:fix` onto it, and then nothing
happened for three days. Measured on that pull request:

- the carried round was refused before Claude ran
- the refusal was posted nowhere
- the rescue round under `conveyor:keep-going` ended in a dispute nobody can answer
- the archive station that follows a merge has no actor at all

The line also gives a person no way to read where a change is, short of
opening run logs.

## What Changes

- **A carried round runs.** The fixing job accepts a run dispatched by the
  repository's own workflow bot, which is how every carried `conveyor:fix`
  round starts. The gate already re-checks the standing grant before that job
  runs, and it keeps doing so.
- **A dead round is reported.** When the fixing step fails, the landing job
  still runs and posts one summary naming the failure and the run, and the
  pull request's loop state says it stalled.
- **A disputed review verdict can be answered.** The review's open-thread gate
  moves from the review workflow into CI's `review-clean` job and is read
  live, and resolving or unresolving a review thread re-runs that job of the
  head's CI run. A person dismissing a finding turns `ci-green` green without
  a hand re-run.
- **The archive station has an actor.** A carried `conveyor:archive` on the
  tracking issue fires a remote session for the archive station, which
  archives the change on its branch and opens the archive pull request with
  `Closes #<n>`. The archive pull request is driven to mergeable under the
  same standing instruction as the first one. A person still merges.
- **The line's progress is readable.** Two state vocabularies, distinct from
  the grant labels, moved by the workflows at every transition: a station
  label on the issue (which station the change is at) and a loop label on the
  pull request (running, stalled, capped, mergeable). Alternatives for a
  second surface are listed in the design for the maintainer to choose from.
- **BREAKING for nobody, but a contract correction:** `remote-change-sessions`
  said the change "is archived by a person on the branch as today". Under
  `conveyor:run` it is archived by a session, and the spec says so.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `conveyor-lifecycle`: four requirements move.
  - The line's state is its labels: two named state vocabularies beside the
    grant labels, moved at every transition.
  - The fixing loop reports a round that could not run, and acts on a run its
    own workflow dispatched.
  - The archive station runs unattended under the standing instruction.
  - A dispute the loop cannot settle is answerable by resolving the thread.
- `remote-change-sessions`: two requirements move.
  - A label on the issue starts a session for the station the label names,
    `conveyor:archive` included, and a carried label is accepted after
    re-checking the grant.
  - The loop's end no longer says a person archives.

## Impact

**Workflows and scripts under `.github/`.** Every one lands with its test in
`.github/tests/`. New labels are created in the repository once, by hand.

| File | Change |
|---|---|
| `workflows/review-dispatch.yml` | the fix job's bot allowlist, land on fix failure, loop state labels |
| `workflows/claude-review.yml` | reconcile no longer fails on open threads |
| `workflows/ci.yml` | `review-clean` reads the threads live |
| `workflows/review-thread.yml` | NEW: re-runs `review-clean` on thread resolution |
| `workflows/remote-implement.yml` | fires on `conveyor:archive`, moves station labels |
| `scripts/remote-implement.py`, `carry-grant.py`, `carry-from-pr.sh`, `land-dispatch.py` | the stations and states above |
| `scripts/conveyor-state.py`, `scripts/rerun-review-clean.py` | NEW |
| `review-triage.json` | the state vocabularies |
| `routines/archive-change.md` | NEW: the archive station's instructions |
| `routines/implement-issue.md` | the station switch at the top |

**Reference docs made untrue and updated:** `.claude/rules/worktree-delivery.md`
(the review's gate, the label table), `.claude/rules/remote-session.md` (the
label table, the archive station), `.claude/rules/gotchas.md` (the #201 note
gains the bot-allowlist measurement), `CONTRIBUTING.md` (the conveyor
section), `docs/testing.md` (what `review-clean` decides), the comments in
`.github/review-triage.json`, and both diagrams under `docs/diagrams/`
(`conveyor-lifecycle.mmd`, `conveyor-lifecycle-implementation.mmd`, new).

**Adopter site:** `docs/security.md` names `conveyor:fix` as the consent the
fixing step acts under, and gains the bot-allowlist bound. No other site page
describes the conveyor, and none of the landing page, Introduction, Getting
started or Installation is affected.

**Not affected:** the manager, the chart, every CRD, every image. Nothing a
cluster decides changes.
