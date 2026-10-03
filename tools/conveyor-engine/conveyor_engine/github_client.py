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


@dataclasses.dataclass(frozen=True)
class PullRequestInfo:
    """A pull request's own state, as a guard predicate needs it.

    `merge_state_status` is GitHub's own computed field, read verbatim
    rather than the bare `mergeable` boolean -- `mergeable` answers only "no
    merge conflict" and says nothing about a failing required check or an
    open required review thread, both of which `mergeStateStatus` already
    folds in. `"CLEAN"` is the one value that means "may merge now".

    `checks_running` is true while any check run GitHub reports for the
    head is still queued or in progress -- distinct from a conclusion,
    which only a COMPLETED run has.
    """

    subject: Subject
    state: str  # "OPEN" | "MERGED" | "CLOSED"
    merged: bool
    merge_state_status: str
    checks_running: bool


@dataclasses.dataclass(frozen=True)
class Comment:
    """One comment on an issue or pull request, as a marker-counting guard
    predicate needs it."""

    created_at: str
    body: str
    is_bot: bool = False


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

    @abc.abstractmethod
    def permission(self, repo: str, login: str) -> str:
        """`login`'s own collaborator permission on `repo`, verbatim as
        GitHub reports it (e.g. `"admin"`, `"write"`, `"read"`, `"none"`)."""

    @abc.abstractmethod
    def open_pull_request_branches(self, repo: str) -> list[str]:
        """The head branch name of every currently OPEN pull request on
        `repo`."""

    @abc.abstractmethod
    def pull_request_info(self, subject: Subject) -> PullRequestInfo:
        """`subject`'s own state -- see `PullRequestInfo`."""

    @abc.abstractmethod
    def branch_check_conclusion(self, repo: str, branch: str) -> str:
        """The aggregate conclusion of `branch`'s latest check runs:
        `"success"`, `"failure"`, `"pending"` (still running), or `""` (no
        check run found)."""

    @abc.abstractmethod
    def list_comments(self, subject: Subject) -> list[Comment]:
        """Every comment on `subject`, oldest first."""


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

    def permission(self, repo: str, login: str) -> str:
        result = subprocess.run(
            [
                "gh",
                "api",
                f"repos/{repo}/collaborators/{login}/permission",
                "--jq",
                ".permission",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        return result.stdout.strip()

    def open_pull_request_branches(self, repo: str) -> list[str]:
        result = subprocess.run(
            [
                "gh",
                "pr",
                "list",
                "--repo",
                repo,
                "--state",
                "open",
                "--json",
                "headRefName",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        data = json.loads(result.stdout)
        return [item["headRefName"] for item in data]

    # Any of these states means the run has not yet settled, across both
    # the Checks API's own vocabulary and the GraphQL `statusCheckRollup`
    # one `gh pr view --json` reports -- there is no single shared name.
    _RUNNING_STATUSES = frozenset(
        {"QUEUED", "IN_PROGRESS", "PENDING", "WAITING", "REQUESTED"}
    )

    def pull_request_info(self, subject: Subject) -> PullRequestInfo:
        result = subprocess.run(
            [
                "gh",
                "pr",
                "view",
                str(subject.number),
                *self._repo_args(subject),
                "--json",
                "state,merged,mergeStateStatus,statusCheckRollup",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        data = json.loads(result.stdout)
        rollup = data.get("statusCheckRollup") or []
        checks_running = any(
            (item.get("status") or "").upper() in self._RUNNING_STATUSES
            for item in rollup
        )
        return PullRequestInfo(
            subject=subject,
            state=data.get("state", ""),
            merged=bool(data.get("merged", False)),
            merge_state_status=data.get("mergeStateStatus", ""),
            checks_running=checks_running,
        )

    def branch_check_conclusion(self, repo: str, branch: str) -> str:
        # `-f`/`-F` on this endpoint are sent as a request BODY, which turns
        # a bodied GET into a plain 404 rather than a permissions error --
        # `--method GET` must be explicit (see this repository's own
        # gotchas.md on `commits/<sha>/check-runs`).
        result = subprocess.run(
            [
                "gh",
                "api",
                f"repos/{repo}/commits/{branch}/check-runs",
                "--method",
                "GET",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        data = json.loads(result.stdout)
        runs = data.get("check_runs") or []
        if not runs:
            return ""
        if any(run.get("status") != "completed" for run in runs):
            return "pending"
        if any(run.get("conclusion") == "failure" for run in runs):
            return "failure"
        return "success"

    def list_comments(self, subject: Subject) -> list[Comment]:
        endpoint = f"repos/{subject.repo or self.repo}/issues/{subject.number}/comments"
        result = subprocess.run(
            ["gh", "api", endpoint, "--method", "GET", "--paginate"],
            capture_output=True,
            text=True,
            check=True,
        )
        data = json.loads(result.stdout)
        return [
            Comment(
                created_at=item.get("created_at", ""),
                body=item.get("body", ""),
                is_bot=(item.get("user") or {}).get("type") == "Bot",
            )
            for item in data
        ]
