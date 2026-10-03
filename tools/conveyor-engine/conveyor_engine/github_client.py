"""The one GitHub-talking boundary every layer above it is built against.

`Subject` names an issue or pull request this engine acts on. `GitHubClient`
is the abstract interface the state writer -- and, in a later section,
every guard predicate -- reads and writes through. A real implementation
may shell out to `gh` (`GhCliClient`, below); every test in this package's
own suite injects a stub with no network at all, per this change's own
rule (no caller is wired in production here).

Build a guard predicate in a later section against this SAME `GitHubClient`
protocol, extending `GhCliClient` with whatever read it needs, rather than
inventing a second GitHub-talking boundary.
"""
from __future__ import annotations

import abc
import dataclasses
import json
import subprocess
from typing import Literal, Optional

SubjectKind = Literal["issue", "pull_request"]


@dataclasses.dataclass(frozen=True)
class Subject:
    """An issue or pull request the engine reads or writes labels on.

    `repo` is `"owner/name"`. Left empty, a `GhCliClient` falls back to
    whatever repository it was constructed with (ordinarily `gh`'s own
    ambient repo, when run from a checkout).
    """

    kind: SubjectKind
    number: int
    repo: str = ""

    def __str__(self) -> str:
        suffix = f" ({self.repo})" if self.repo else ""
        return f"{self.kind}#{self.number}{suffix}"


class GitHubClient(abc.ABC):
    """The boundary every caller above it is injected with, never imports
    directly. A test stub implements this with no network at all."""

    @abc.abstractmethod
    def get_labels(self, subject: Subject) -> set[str]:
        """Every label currently on `subject`."""

    @abc.abstractmethod
    def set_labels(self, subject: Subject, labels: set[str]) -> None:
        """Replace `subject`'s label set with exactly `labels`."""

    @abc.abstractmethod
    def related_pull_requests(self, issue: Subject) -> list[Subject]:
        """Every pull request GitHub considers related to `issue`."""

    @abc.abstractmethod
    def related_issue(self, pull_request: Subject) -> Optional[Subject]:
        """The issue GitHub considers related to `pull_request`, if any."""


class GhCliClient(GitHubClient):
    """Shells out to `gh`. Not exercised by this package's own test suite
    (no network there, by this change's own rule) and not wired into any
    production caller in this change -- kept here only so the next layer
    (guard predicates) has one real implementation of this boundary to
    extend, rather than a second one being invented beside it.

    `related_pull_requests` / `related_issue` read GitHub's own
    cross-reference timeline, which is best-effort: GitHub exposes no
    single "linked pull requests" query for an arbitrary issue.
    """

    def __init__(self, repo: Optional[str] = None):
        self.repo = repo

    def _repo_args(self, subject: Subject) -> list[str]:
        repo = subject.repo or self.repo
        return ["--repo", repo] if repo else []

    def get_labels(self, subject: Subject) -> set[str]:
        kind = "issue" if subject.kind == "issue" else "pr"
        result = subprocess.run(
            [
                "gh",
                kind,
                "view",
                str(subject.number),
                *self._repo_args(subject),
                "--json",
                "labels",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        data = json.loads(result.stdout)
        return {label["name"] for label in data.get("labels", [])}

    def set_labels(self, subject: Subject, labels: set[str]) -> None:
        endpoint = f"repos/{subject.repo or self.repo}/issues/{subject.number}"
        subprocess.run(
            ["gh", "api", endpoint, "-X", "PATCH", "--input", "-"],
            input=json.dumps({"labels": sorted(labels)}),
            text=True,
            check=True,
            capture_output=True,
        )

    def related_pull_requests(self, issue: Subject) -> list[Subject]:
        repo = issue.repo or self.repo
        result = subprocess.run(
            [
                "gh",
                "issue",
                "view",
                str(issue.number),
                *self._repo_args(issue),
                "--json",
                "timelineItems",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        data = json.loads(result.stdout)
        numbers = {
            item["source"]["number"]
            for item in data.get("timelineItems", [])
            if item.get("__typename") == "CrossReferencedEvent"
            and item.get("source", {}).get("number")
        }
        return [
            Subject(kind="pull_request", number=number, repo=repo)
            for number in sorted(numbers)
        ]

    def related_issue(self, pull_request: Subject) -> Optional[Subject]:
        repo = pull_request.repo or self.repo
        result = subprocess.run(
            [
                "gh",
                "pr",
                "view",
                str(pull_request.number),
                *self._repo_args(pull_request),
                "--json",
                "closingIssuesReferences",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        data = json.loads(result.stdout)
        refs = data.get("closingIssuesReferences") or []
        if not refs:
            return None
        return Subject(kind="issue", number=refs[0]["number"], repo=repo)
