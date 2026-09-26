## Why

On #259 (2026-09-26) the fixing loop ended after two rounds with `docs-task`
and `ci-green` red, posted "clean" and set `loop:stalled`. A
`conveyor:keep-going` round an hour later repeated the same words.

- The red was the loop's own dispute guard.
- The dispute was one the landing step had manufactured from a fix that
  claimed to land and did not.
- Once the person resolved the thread, nothing re-read it.

The rule this change states: **a red pull request is the loop's to fix**, and
a person is owed nothing until it is green, or until every remaining item is
a dispute the fixing step actually made.

## What Changes

- **The dispute guard leaves CI.** `docs-task` no longer fails on an
  unanswered dispute. The open thread already holds the merge through
  required conversation resolution, live. The `openspec archive` refusal
  stays, in the hook.
- **`dispute-answered.yml` and `rerun-ci-job.py` are retired.** No required
  check reads a person's answer any more, so nothing has to be re-run on it.
  A person's comment on a waiting pull request starts a round instead.
- **Every failed required check is work.** The "failed only on the loop's
  own guard" exemption in `check_is_work` is deleted with the guard that made
  it necessary.
- **A claimed fix that landed nothing is UNADDRESSED, never a dispute.** The
  thread stays untouched and the item stays eligible. A dispute is a thing
  the fixing step says, not a thing the landing step infers.
- **A round that lands nothing while items remain eligible starts the next
  round itself**, within the bound. Today it ends the loop.
- **`loop:waiting` is a state of its own.** Entered when every remaining item
  is a dispute awaiting a person. `loop:stalled` narrows to the machine
  stopping: no report, fixer failed or timed out, next round could not start.
  A stale patch is a retry, not a stop.
- **A waiting loop is left two ways.** A comment from a person with write
  access on the pull request starts a round. A scheduled sweep re-reads
  waiting pull requests and starts a round once no dispute is unanswered,
  because a thread resolution fires no workflow event.
- **A round starts on a head only once its CI run and its review run have
  both concluded.** The checks are "consulted" when every required check run
  has completed, not when it merely exists. On #259 a round declared "0
  failures" 38 seconds into a ten-minute CI run.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `conveyor-lifecycle`: the loop's state vocabulary gains `waiting`. No
  required check carries the loop's conversation, dispute included, and the
  re-run-on-answer requirement is removed. Every red on the head is work. A
  round starts only on a concluded head. A waiting loop is resumed by a
  comment or a sweep.
- `automated-code-review`: a claimed fix without evidence is unaddressed. A
  round that changes nothing continues while items remain eligible. The
  checks are consulted only once concluded.
- `change-delivery`: the archive refusal over an open loop is the archive
  command's alone, never a check's.

## Impact

Repository tooling only. No manager, chart, runtime or cluster behaviour
changes.

**Code.** `.github/scripts/conveyor.py`, `failed-checks.py`,
`land-dispatch.py`, `accepted-findings.py`, `autofix-guard.py`,
`review-triage.json`, `retired-vocabulary.json`. Workflows `ci.yml`,
`review-dispatch.yml`, a new `conveyor-sweep.yml`. Deleted:
`dispute-answered.yml`, `rerun-ci-job.py` and their tests. Every touched
script's test under `.github/tests/`.

**Reference docs made untrue.** `CONTRIBUTING.md` (the loop section: the
`docs-task` re-run paragraph, the label list, when the next round starts).
`docs/testing.md` (the row naming `dispute-answered.yml`).
`docs/diagrams/conveyor-lifecycle-implementation.mmd` (the stalled node, the
answer path). `docs/CHANGELOG.md`. `.claude/rules/worktree-delivery.md` (the
label table and the review-found-something section),
`.claude/rules/documentation.md` (the hook row), `.claude/rules/gotchas.md`
(the #259 measurement).

**Adopter site.** No page describes the fixing loop to an adopter. The
landing page, `introduction.md`, `getting-started.md`, `installation.md` and
`docs/guides/*` are checked and recorded as unaffected in the documentation
task. `docs/security.md` names the conveyor once and is re-read.
