"""Shared no-network `GitHubClient` test doubles.

Every test in this suite injects one of these instead of talking to a real
`gh`, per this change's own rule (no network anywhere in the test suite).
"""
from __future__ import annotations

from typing import Optional

from conveyor_engine.github_client import Comment, GitHubClient, PullRequestInfo, Subject


class FakeClient(GitHubClient):
    """An in-memory GitHub: a dict of subject -> label set, plus an
    optional issue<->pull-request relation, with no network at all.

    `fail_on` names a method that raises `RuntimeError` instead of acting,
    for tests of the "a write fails" / "a read is unreadable" path.
    """

    def __init__(
        self,
        labels: Optional[dict[Subject, set[str]]] = None,
        related: Optional[dict[Subject, list[Subject]]] = None,
        fail_on: Optional[str] = None,
        permissions: Optional[dict[tuple[str, str], str]] = None,
        open_branches: Optional[dict[str, list[str]]] = None,
        pr_infos: Optional[dict[Subject, PullRequestInfo]] = None,
        branch_conclusions: Optional[dict[tuple[str, str], str]] = None,
        comments: Optional[dict[Subject, list[Comment]]] = None,
    ):
        self.labels: dict[Subject, set[str]] = {
            k: set(v) for k, v in (labels or {}).items()
        }
        # issue -> [pull_request, ...]; also indexes the reverse direction.
        self.related: dict[Subject, list[Subject]] = {
            k: list(v) for k, v in (related or {}).items()
        }
        self.fail_on = fail_on
        # (repo, login) -> permission string.
        self.permissions: dict[tuple[str, str], str] = dict(permissions or {})
        # repo -> [headRefName, ...].
        self.open_branches: dict[str, list[str]] = {
            k: list(v) for k, v in (open_branches or {}).items()
        }
        self.pr_infos: dict[Subject, PullRequestInfo] = dict(pr_infos or {})
        # (repo, branch) -> conclusion string.
        self.branch_conclusions: dict[tuple[str, str], str] = dict(
            branch_conclusions or {}
        )
        self.comments: dict[Subject, list[Comment]] = {
            k: list(v) for k, v in (comments or {}).items()
        }

    def _maybe_fail(self, method: str) -> None:
        if self.fail_on == method:
            raise RuntimeError(f"simulated failure in {method}")

    def get_labels(self, subject: Subject) -> set[str]:
        self._maybe_fail("get_labels")
        return set(self.labels.get(subject, set()))

    def set_labels(self, subject: Subject, labels: set[str]) -> None:
        self._maybe_fail("set_labels")
        self.labels[subject] = set(labels)

    def related_pull_requests(self, issue: Subject) -> list[Subject]:
        self._maybe_fail("related_pull_requests")
        return list(self.related.get(issue, []))

    def related_issue(self, pull_request: Subject) -> Optional[Subject]:
        self._maybe_fail("related_issue")
        for issue, prs in self.related.items():
            if pull_request in prs:
                return issue
        return None

    def permission(self, repo: str, login: str) -> str:
        self._maybe_fail("permission")
        return self.permissions.get((repo, login), "none")

    def open_pull_request_branches(self, repo: str) -> list[str]:
        self._maybe_fail("open_pull_request_branches")
        return list(self.open_branches.get(repo, []))

    def pull_request_info(self, subject: Subject) -> PullRequestInfo:
        self._maybe_fail("pull_request_info")
        if subject not in self.pr_infos:
            raise KeyError(f"no fake PullRequestInfo registered for {subject}")
        return self.pr_infos[subject]

    def branch_check_conclusion(self, repo: str, branch: str) -> str:
        self._maybe_fail("branch_check_conclusion")
        return self.branch_conclusions.get((repo, branch), "")

    def list_comments(self, subject: Subject) -> list[Comment]:
        self._maybe_fail("list_comments")
        return list(self.comments.get(subject, []))


class RecordingClient(GitHubClient):
    """Records every call it receives; never raises. Used to prove an
    `owned_by` stub never touches a client made available to it."""

    def __init__(self):
        self.calls: list[tuple[str, tuple]] = []

    def get_labels(self, subject: Subject) -> set[str]:
        self.calls.append(("get_labels", (subject,)))
        return set()

    def set_labels(self, subject: Subject, labels: set[str]) -> None:
        self.calls.append(("set_labels", (subject, labels)))

    def related_pull_requests(self, issue: Subject) -> list[Subject]:
        self.calls.append(("related_pull_requests", (issue,)))
        return []

    def related_issue(self, pull_request: Subject) -> Optional[Subject]:
        self.calls.append(("related_issue", (pull_request,)))
        return None

    def permission(self, repo: str, login: str) -> str:
        self.calls.append(("permission", (repo, login)))
        return "none"

    def open_pull_request_branches(self, repo: str) -> list[str]:
        self.calls.append(("open_pull_request_branches", (repo,)))
        return []

    def pull_request_info(self, subject: Subject) -> PullRequestInfo:
        self.calls.append(("pull_request_info", (subject,)))
        return PullRequestInfo(
            subject=subject,
            state="OPEN",
            merged=False,
            merge_state_status="",
            checks_running=False,
        )

    def branch_check_conclusion(self, repo: str, branch: str) -> str:
        self.calls.append(("branch_check_conclusion", (repo, branch)))
        return ""

    def list_comments(self, subject: Subject) -> list[Comment]:
        self.calls.append(("list_comments", (subject,)))
        return []
