"""Guard predicate factories that read real `GitHubClient` state.

Every function here is a FACTORY: given a `GitHubClient` and whatever
subject or scalar it needs, it returns a zero-argument closure suitable
for a `guards.GuardRegistry` entry (see `engine.py`'s own class docstring
-- a registered predicate takes no argument, and whoever builds one is
responsible for closing it over its own context).

Each factory wraps its real read in `_guarded`, which catches any
exception the client raises and returns the predicate's own FAIL-CLOSED
answer instead -- the direction that treats the subject as already at
work, already granted, or already carrying whatever fact a wrong answer
would otherwise start unattended action on (conveyor-engine spec, "A guard
predicate fails closed"). Each predicate below states, in its own
docstring, which direction that is for it.
"""
from __future__ import annotations

from typing import Callable

from .github_client import GitHubClient, Subject

_WRITE_PERMISSIONS = frozenset({"admin", "maintain", "write"})

# `mergeStateStatus` values that mean "nothing is blocking a merge right
# now" -- the one value read, everywhere this module asks "is this pull
# request mergeable" (see `PullRequestInfo`'s own docstring on why this is
# `mergeStateStatus` and never the bare `mergeable` boolean).
_MERGEABLE_STATUS = "CLEAN"


def _guarded(read: Callable[[], bool], fail_closed: bool) -> Callable[[], bool]:
    """Wrap a real read so any exception resolves to `fail_closed` instead
    of propagating -- the one place every predicate below applies its own
    fail-closed rule."""

    def predicate() -> bool:
        try:
            return bool(read())
        except Exception:  # noqa: BLE001 -- deliberately broad: any read failure
            return fail_closed

    return predicate


def has_access(client: GitHubClient, repo: str, login: str) -> Callable[[], bool]:
    """True when `login` holds `admin`, `maintain` or `write` on `repo`.

    FAILS CLOSED TOWARD NO ACCESS (False) -- an unreadable permission must
    never be read as "may push here".
    """

    def read() -> bool:
        return client.permission(repo, login) in _WRITE_PERMISSIONS

    return _guarded(read, fail_closed=False)


def is_session_at_work(client: GitHubClient, repo: str, branch: str) -> Callable[[], bool]:
    """True when a pull request is already open from `branch`.

    FAILS CLOSED TOWARD A SESSION ALREADY AT WORK (True) -- an unreadable
    pull-request list must never be read as "nothing is running", which
    would start a second session on the same branch.
    """

    def read() -> bool:
        return branch in client.open_pull_request_branches(repo)

    return _guarded(read, fail_closed=True)


def has_open_prs(client: GitHubClient, issue: Subject) -> Callable[[], bool]:
    """True when at least one pull request related to `issue` is still
    OPEN.

    FAILS CLOSED TOWARD "NOT YET" (False) -- an unreadable related-pull-
    request list must never be read as "pull requests already exist".
    """

    def read() -> bool:
        related = client.related_pull_requests(issue)
        return any(
            client.pull_request_info(pr).state == "OPEN" for pr in related
        )

    return _guarded(read, fail_closed=False)


def all_prs_mergeable(client: GitHubClient, issue: Subject) -> Callable[[], bool]:
    """True when every pull request related to `issue` reads
    `mergeStateStatus == "CLEAN"`.

    An issue with NO related pull request is NOT mergeable -- there is
    nothing to merge, so the honest answer is "not yet" rather than a
    vacuous true.

    FAILS CLOSED TOWARD "NOT YET" (False).
    """

    def read() -> bool:
        related = client.related_pull_requests(issue)
        if not related:
            return False
        return all(
            client.pull_request_info(pr).merge_state_status == _MERGEABLE_STATUS
            for pr in related
        )

    return _guarded(read, fail_closed=False)


def pr_is_mergeable(client: GitHubClient, pull_request: Subject) -> Callable[[], bool]:
    """True when `pull_request` itself reads `mergeStateStatus == "CLEAN"`.

    The same underlying read backs `proposal_pr_is_mergeable` below, closed
    over whichever subject is the one actually being asked about.

    FAILS CLOSED TOWARD "NOT YET" (False).
    """

    def read() -> bool:
        return client.pull_request_info(pull_request).merge_state_status == _MERGEABLE_STATUS

    return _guarded(read, fail_closed=False)


# `conveyor.propose`'s own guard name for the identical read: the proposal
# pull request the invoked `loop` workflow is tracking. Named separately in
# the registry for clarity at the call site, never a second implementation.
proposal_pr_is_mergeable = pr_is_mergeable


def all_prs_merged(client: GitHubClient, issue: Subject) -> Callable[[], bool]:
    """True when every pull request related to `issue` has `merged` set.

    An issue with no related pull request is NOT "all merged" -- same
    vacuous-truth reasoning as `all_prs_mergeable`.

    FAILS CLOSED TOWARD "NOT YET" (False).
    """

    def read() -> bool:
        related = client.related_pull_requests(issue)
        if not related:
            return False
        return all(client.pull_request_info(pr).merged for pr in related)

    return _guarded(read, fail_closed=False)


def master_is_green(client: GitHubClient, repo: str, branch: str = "master") -> Callable[[], bool]:
    """True when `branch`'s latest check runs all concluded `success`.

    FAILS CLOSED TOWARD "NOT YET" (False) -- an unreadable or absent
    conclusion must never be read as green.
    """

    def read() -> bool:
        return client.branch_check_conclusion(repo, branch) == "success"

    return _guarded(read, fail_closed=False)


def hotfix_pr_is_created(client: GitHubClient, pull_request: Subject) -> Callable[[], bool]:
    """True when `pull_request` (the hotfix pull request `conveyor.fix`'s
    own transition names) exists and is readable, open or merged.

    FAILS CLOSED TOWARD "ALREADY CREATED" (True) -- a wrong "not created"
    would start a duplicate hotfix pull request unattended.
    """

    def read() -> bool:
        return client.pull_request_info(pull_request).state in ("OPEN", "MERGED")

    return _guarded(read, fail_closed=True)


def all_checks_ran(client: GitHubClient, pull_request: Subject) -> Callable[[], bool]:
    """True when `pull_request`'s own checks are no longer running --
    every one has a terminal status.

    FAILS CLOSED TOWARD "NOT YET" (False) -- an unreadable check list must
    never be read as "done".
    """

    def read() -> bool:
        return not client.pull_request_info(pull_request).checks_running

    return _guarded(read, fail_closed=False)
