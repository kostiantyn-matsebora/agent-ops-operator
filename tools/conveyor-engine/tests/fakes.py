"""Shared no-network `GitHubClient` test doubles.

Every test in this suite injects one of these instead of talking to a real
`gh`, per this change's own rule (no network anywhere in the test suite).
"""
from __future__ import annotations

from typing import Optional

from conveyor_engine.github_client import GitHubClient, Subject


class FakeClient(GitHubClient):
    """An in-memory GitHub: a dict of subject -> label set, plus an
    optional issue<->pull-request relation, with no network at all.

    `fail_on` names a method that raises `RuntimeError` instead of acting,
    for tests of the "a write fails" path.
    """

    def __init__(
        self,
        labels: Optional[dict[Subject, set[str]]] = None,
        related: Optional[dict[Subject, list[Subject]]] = None,
        fail_on: Optional[str] = None,
    ):
        self.labels: dict[Subject, set[str]] = {
            k: set(v) for k, v in (labels or {}).items()
        }
        # issue -> [pull_request, ...]; also indexes the reverse direction.
        self.related: dict[Subject, list[Subject]] = {
            k: list(v) for k, v in (related or {}).items()
        }
        self.fail_on = fail_on

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
