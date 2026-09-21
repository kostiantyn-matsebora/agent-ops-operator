## Why

Measured live on #222: its pull request (#226) merged on 2026-09-19, but the
issue never carried `conveyor:run` — worked by hand-triggered sessions before
the label vocabulary existed on it. Nothing ever carried the line onward.

It sat labelled `station:merge` for two days. That label's own description
says "the pull request is mergeable and waits for a person" — already false
the moment #226 merged.

Nothing in the conveyor treats this as an event worth acting on.
`carry-grant.py --station archive` exits 0 doing nothing when `conveyor:run`
is absent — correct everywhere else it is called.

This one call only ever runs after a merge, the archive job's own trigger.
An opsx-lane issue with nothing to carry at that point is stalled, not
merely ungranted.

## What Changes

- **A merge with nothing to carry it marks the line stalled.**
  `carry-grant.py --station archive`, finding no `conveyor:run` on an
  opsx-lane issue, now sets `station:stalled` instead of leaving the prior
  `station:merge` label in place describing a precondition that has already
  resolved.
- **A new state label**, `station:stalled`, alongside the existing
  `station:*` family — created once, by hand, like the others.
- **The plain lane is unaffected.** It has no archive station, so this only
  fires for an opsx-lane issue.
- **Nothing new is granted.** `station:stalled` is state, not a grant, exactly
  like every other station label — a person reads it and decides whether to
  place `conveyor:run` (or the single-station `conveyor:archive`) to resume
  the line, or leave it.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `conveyor-lifecycle`: two requirements move.
  - A merge with no standing instruction to carry moves the issue's station
    to stalled, not left on the merge label.
  - A station's label must stay true to its own stated meaning for as long
    as it is shown.

## Impact

**Scripts under `.github/`.**

| File | Change |
|---|---|
| `scripts/carry-grant.py` | sets `station:stalled` when `--station archive` finds no `conveyor:run` on an opsx-lane issue |
| `review-triage.json` | new `station_labels.stalled` entry, comment updated |
| `tests/carry-grant.test.sh` | three new cases: opsx-lane archive with nothing to carry, plain-lane archive with nothing to carry (unaffected), and `--station fix` with nothing to carry (unaffected) |

**Reference docs made untrue and updated:** none beyond
`.claude/rules/worktree-delivery.md`'s and `.claude/rules/remote-session.md`'s
station-label tables, which gain the new value.

**Adopter site:** not affected. This is repository-internal automation with
no CRD, contract, chart or adopter-visible behaviour.

**Not affected:** the manager, the chart, every CRD, every image, every
runtime. Nothing a cluster decides changes.
