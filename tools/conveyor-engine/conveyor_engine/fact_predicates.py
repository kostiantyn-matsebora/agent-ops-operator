"""Guard predicate factories over a fact the CALLER already knows.

Two groups of guard, both built the same way as thin wrappers rather than
fresh reads through `GitHubClient`:

`loop`'s own round bookkeeping (`is_capped`, `reset`) and the two round-
completion facts (`session_finished`, `checks_completed`) a `round_finished`
event already carries by the time it fires -- the event exists because a
round's own fixing job and its checks already concluded, so a predicate
re-deriving that from a fresh API read would be re-stating a fact its own
caller is holding.

`review`'s own three guards (`review_run_succeeded`, `has_open_review_
threads`, `review_run_skipped`) by design.md Decision 7: they read facts
from a run that ALREADY HAPPENED, supplied by whatever reads
`claude-review.yml`'s own finished run -- never re-derived here. This is
the one guard group the design explicitly calls out as "thin wrappers",
and this module extends that same shape to the two `loop` facts above
for the same reason: nothing in this change wires a real caller for
either group (design.md's Non-Goals), so inventing a fresh
`GitHubClient`-backed read for `session_finished` / `checks_completed`
would be guessing at a shape the follow-up change, which DOES wire a
caller, is better placed to settle. See this task's own report for the
judgment call named explicitly.

Every factory here takes `Optional[...]` -- `None` means "the fact could
not be read" -- and returns the fail-closed answer the conveyor-engine
spec's "A guard predicate fails closed" requirement names. None of these
functions can raise: there is no I/O here to fail.
"""
from __future__ import annotations

from typing import Callable, Optional


def cap_for(max_rounds: int, grants: int) -> int:
    """The effective ceiling: each consumed grant adds a full set of
    `max_rounds` -- the same formula this repository's own
    `conveyor.py:cap_for` states, re-derived rather than imported (see this
    package's own rule against importing `conveyor.py`)."""
    return max_rounds * (1 + grants)


def count_marker_occurrences(
    comments: list, marker: str, since: str = ""
) -> int:
    """How many comments carry `marker` as one of their OWN LINES (never
    merely as a substring somewhere in the body -- a quoted reply that
    only mentions the marker must not count), counting only those created
    at or after `since`.

    Takes anything with `.created_at` and `.body` attributes (a
    `github_client.Comment`, ordinarily) so a test can pass plain stand-ins
    with no client at all. This is a pure, no-I/O helper: the real read is
    `GitHubClient.list_comments`, and wiring the two together into a single
    "count rounds since the fix label was placed" fact for `is_capped` /
    `reset` is left to whichever caller resolves `since` and `marker` in
    production -- see this task's own report for why this stays a thin
    helper rather than a third factory in this module.
    """
    return sum(
        1
        for comment in comments
        if marker in comment.body.splitlines()
        and (not since or comment.created_at >= since)
    )


def is_capped(rounds_used: Optional[int], grants: int, max_rounds: int) -> Callable[[], bool]:
    """True when `rounds_used` has reached (or passed) `cap_for(max_rounds,
    grants)`.

    FAILS CLOSED TOWARD ALREADY CAPPED (True) when `rounds_used` is
    unreadable -- the guard gates `round:fixing_failed`'s own automatic
    retry (`NOT is_capped`), so treating an unreadable count as "room
    left" would let the loop keep firing unattended work past a bound
    nobody could verify.
    """

    def predicate() -> bool:
        if rounds_used is None:
            return True
        return rounds_used >= cap_for(max_rounds, grants)

    return predicate


def reset(granted: Optional[bool]) -> Callable[[], bool]:
    """True when a `loop:restart` grant was given.

    FAILS CLOSED TOWARD NOT GRANTED (False) -- an unreadable grant must
    never be read as a person's own instruction to restart.
    """

    return lambda: bool(granted)


def session_finished(finished: Optional[bool]) -> Callable[[], bool]:
    """True when the round's own fixing session has concluded, whatever
    its outcome.

    FAILS CLOSED TOWARD NOT FINISHED (False) -- `round:fixing`'s own
    transition only decides `round:mergeable` vs. `round:fixing_failed`
    once every one of its guard's clauses is true, so treating an
    unreadable session state as "finished" could move the state while the
    session is still running.
    """

    return lambda: bool(finished)


def checks_completed(completed: Optional[bool]) -> Callable[[], bool]:
    """True when the round's own checks have all reached a terminal
    status.

    FAILS CLOSED TOWARD NOT COMPLETED (False), for the same reason as
    `session_finished`.
    """

    return lambda: bool(completed)


def review_run_succeeded(conclusion: Optional[str]) -> Callable[[], bool]:
    """True when the finished review run's own conclusion was `"success"`.

    FAILS CLOSED TOWARD "NOT LANDED" (False) -- an unreadable conclusion
    must never be read as a clean run.
    """

    return lambda: conclusion == "success"


def has_open_review_threads(thread_count: Optional[int]) -> Callable[[], bool]:
    """True when the pull request's open review-thread count is greater
    than zero.

    FAILS CLOSED TOWARD THREADS STILL OPEN (True) when the count is
    unreadable -- an unreadable thread count must never be read as
    `scan:clean`.
    """

    def predicate() -> bool:
        if thread_count is None:
            return True
        return thread_count > 0

    return predicate


def review_run_skipped(skipped: Optional[bool]) -> Callable[[], bool]:
    """True when the run's own `queue` job reported a skip.

    FAILS CLOSED TOWARD NOT SKIPPED (False) -- an unreadable skip flag must
    never be read as `scan:skipped`, which names no failure action at all.
    """

    return lambda: bool(skipped)
