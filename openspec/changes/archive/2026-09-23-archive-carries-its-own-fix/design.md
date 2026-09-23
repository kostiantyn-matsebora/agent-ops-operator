## Context

See proposal.md for the measured case (#237, via #222). What shapes the fix:

- **`carry-grant.py --station fix` is called for every pull request**, opened
  by `conveyor:run` or by any single-station label. It only ever checked
  `run_label` (`conveyor:run`) as the grant authorising the carry.
- **`conveyor:archive` already carries its own re-checked authorisation.**
  Its own `--station archive` call re-verifies the placer's write access at
  the moment it fires the archive session — the fix-station carry needs no
  new trust model, only to recognise that same authorisation reaches the
  pull request the session produced.
- **A program may carry a grant forward or consume one, never mint one.**
  The fallback re-checks `conveyor:archive`'s own placement and permission
  exactly as `conveyor:run`'s are checked today — nothing here trusts a
  label because a workflow already looked at it once.

## Goals / Non-Goals

**Goals:**

- A pull request opened by a hand-placed `conveyor:archive` gets its own
  fixing loop, with no separate grant a person has to place.
- `conveyor:run`'s own behaviour is unchanged when it is present.

**Non-Goals:**

- Changing what `--station archive` itself checks. Only the `fix`-station
  carry gains the fallback.
- Making `conveyor:archive` a second standing instruction. It is still a
  single-station label — the fix is that its one station now includes
  driving its own pull request, matching what the label table already says.

## Decisions

### `carry-grant.py` computes ONE effective grant label, once

Before this change, every check inside `main()` referred to `run_label`
directly. The fix introduces `grant_label`, resolved once near the top.

It is `run_label` if present, else `archive_label` when the call is
`--station fix` and `run_label` is absent. Every later check (label
placement, permission, the success comment) reads `grant_label` instead, so
the same re-verification path serves both grants without a second copy of
it.

- **`conveyor:run` always wins when both are present.** The fallback only
  triggers when `run_label not in labels`, so an opsx-lane issue mid-line
  with a fresh `conveyor:archive` placed for some other reason does not
  silently change which grant a reader sees credited.
- **The fallback is scoped to `--station fix` only.** `--station archive`
  keeps checking `run_label` alone — recognising its own label as its own
  authorisation would be circular, and the existing `station:stalled`
  fallback already covers the no-grant-at-all case there.

### No change to `carry-from-pr.sh` or the workflow

The defect was entirely inside `carry-grant.py`'s own grant check.
`carry-from-pr.sh --station fix` already calls `carry-grant.py --station
fix --pr <n>` unconditionally for every resolved issue — it needed no new
branch, since the fallback is invisible to its caller.

## Risks / Trade-offs

- [A person placing `conveyor:archive` did not expect it to also authorise
  fixing rounds] → the label's own published description
  (`worktree-delivery.md`) already says the loop drives the resulting pull
  request, so this closes a gap between the promise and the code rather
  than making a new promise.
- [Two grants now read as interchangeable for one station] → only for
  `--station fix`, only when `run_label` is absent, and the carried-from
  comment always names which one it was.

## Migration Plan

1. Merge. The next `--station fix` call for a pull request whose issue
   carries `conveyor:archive` (and not `conveyor:run`) carries correctly.
2. #237 itself: re-dispatch (`gh workflow run remote-implement.yml -f
   pr=226`, or push again) to pick up the fix, or place `conveyor:fix` on
   #237 directly — either resumes its line the same way this change makes
   automatic going forward.
