## Context

See proposal.md for the measured case (#222). What shapes the fix:

- **`carry-grant.py --station archive` is only ever called after a merge.**
  `remote-implement.yml`'s `archive` job triggers on `workflow_run` after `ci`
  completes for a `push` to master, or on a manual `workflow_dispatch` replay
  naming a specific merged pull request. Either way, by the time this script
  runs, the merge already happened.
- **`station:merge` is a precondition label, unlike its siblings.** Every
  other station label describes ongoing work ("the line is implementing" /
  "fixing" / "archiving"). `station:merge`'s own description is a fact that
  becomes false at merge — "mergeable, waits for a person".
- **Nothing may mint a grant.** `station:stalled` is state, exactly like
  every other station label — it changes what a person reads, never what a
  program is authorised to do next.

## Goals / Non-Goals

**Goals:**

- A merge with no standing instruction to carry it moves the issue off
  `station:merge` at once, onto a label that reads true.
- The plain lane and the `fix` station are unaffected — this is one path,
  taken only when a merge already happened and there is nothing to carry.

**Non-Goals:**

- Minting a grant. `station:stalled` authorises nothing. A person still
  places `conveyor:run` or `conveyor:archive` to resume the line.
- Retrofitting every issue this has already happened to. #222 is fixed by a
  person placing a label once this change ships, same as any other stalled
  line.
- A general "expire stale labels" mechanism. This is the one label whose
  condition can silently lapse. The others describe ongoing work that a
  later transition already corrects.

## Decisions

### `carry-grant.py` sets `station:stalled`, not nothing, on this one path

Today, `--station archive` finding no `conveyor:run` prints "nothing to
carry" and returns 0, changing no label. That is correct for `--station fix`
and for a plain-lane issue — both cases where nothing established a merge
just happened.

An opsx-lane `--station archive` call is different: a merge is the reason
this program is running at all. "Nothing to carry" there is a line stalled
at the station it just finished, not the ordinary case.

The fix is narrow: inside the existing `run_label not in labels` branch,
check `args.station == "archive" and is_opsx_lane(...)` (the same lane check
already used two lines below for a different purpose) and set
`station:stalled` instead of leaving the label alone.

### `station:stalled`, not a repurposed existing value

`loop:stalled` already names this concept on the pull request side. Reusing
its name on the issue side keeps one word meaning one thing across both
families, rather than inventing a second word for the same idea.

- **Not `station:merge` removed with nothing added.** A person reading the
  issue should see SOMETHING, not an issue with no station label at all,
  which reads as never having entered the line.
- **Not `station:done`.** The line has not ended — it is one grant away
  from continuing, unlike `done`, which means the line finished.

### No workflow change

The fix lives entirely in `carry-grant.py`. `remote-implement.yml`'s
`archive` job already calls it unconditionally on every merge, so this
needs no new trigger, no new job, no new permission.

## Risks / Trade-offs

- [A stalled issue looks like a dead end] → its description and
  `worktree-delivery.md`'s label table both say a person may resume it by
  placing `conveyor:run` or `conveyor:archive` directly.
- [Existing stuck issues are not retroactively fixed] → the next real event
  (a person placing a label) corrects them the same way any station
  transition does. Nothing here is a one-time migration.

## Migration Plan

1. Merge. The next opsx-lane merge with no `conveyor:run` to carry sets
   `station:stalled` instead of leaving `station:merge`.
2. Create the `station:stalled` label in the repository by hand (done ahead
   of this change, alongside the other eight state labels).
3. #222 itself: a person places `conveyor:run` or `conveyor:archive` to
   resume its line, independent of this change shipping.
