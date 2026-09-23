## Why

Measured live on #237: `conveyor:archive` was placed directly on #222 (its
issue had no `conveyor:run` at all), and the archive session fired
correctly and opened #237. But #237's own fixing loop never started — two
review findings sat open with nothing carrying `conveyor:fix` onto the pull
request.

The cause: `carry-from-pr.sh --station fix` checks only `conveyor:run` on
the tracking issue.

`worktree-delivery.md`'s own label table already states the archive
station's promise as one fact — it starts a session that opens the archive
pull request, "and the loop drives that pull request to mergeable". The
code only did the first half.

A person placing `conveyor:archive` by hand is the sanctioned recovery path
for an issue whose standing instruction was never placed, or was consumed.
That placement got a pull request needing the same findings resolved by
hand the whole mechanism exists to avoid.

## What Changes

- **`conveyor:archive` is its own grant for the fix station.**
  `carry-grant.py --station fix`, finding no `conveyor:run` on the issue,
  now checks for `conveyor:archive` instead. If present, its own placer and
  their current write access authorise the carry — the same re-check
  `conveyor:run` already gets, on the label that is actually driving this
  pull request into existence.
- **`conveyor:run` still wins when both are present.** The fallback only
  applies when `conveyor:run` is absent, so an ordinary opsx-lane issue's
  behaviour is unchanged.
- **The published contract catches up to what is already documented.** The
  spec's "a single station is driven by hand" scenario said every
  hand-placed station stops the line there — true for `conveyor:implement`
  and `conveyor:fix`, never true for `conveyor:archive`, whose own row
  already promised otherwise.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `conveyor-lifecycle`: one requirement moves.
  - A hand-placed archive label drives its own resulting pull request to
    mergeable, an explicit exception to the general single-station rule.

## Impact

**Scripts under `.github/`.**

| File | Change |
|---|---|
| `scripts/carry-grant.py` | `--station fix` accepts `conveyor:archive` as a fallback grant when `conveyor:run` is absent |
| `tests/carry-grant.test.sh` | three new cases: the archive-label fallback carries, `conveyor:run` still takes priority when both are present, an archive-label placer who lost write access is refused the same way |

**Reference docs made untrue and updated:**
`docs/diagrams/conveyor-lifecycle-implementation.mmd`'s `CARRY` node named
only `conveyor:run` as the re-checked grant, and gains `conveyor:archive`.
`.claude/rules/worktree-delivery.md`'s `conveyor:archive` row already
states the intended behaviour — this change makes the code match it, not
the other way round.

**Adopter site:** not affected. This is repository-internal automation with
no CRD, contract, chart or adopter-visible behaviour.

**Not affected:** the manager, the chart, every CRD, every image, every
runtime. Nothing a cluster decides changes.
