"""Task 2.8: every bare predicate name `workflows.desired.yaml` uses must
resolve to something this section built.

`conveyor_engine.loader` has no helper for collecting every guard string
across every loaded workflow, so this test writes its own -- a small,
local walk of `Transition.guard`, exactly as the task names as the fallback.
It reuses `conveyor_engine.guards`' own tokenizer (`_TOKEN_RE`) rather than
writing a second one, so "what counts as a bare name" can never drift
between the real evaluator and this coverage check.
"""
from __future__ import annotations

import unittest
from pathlib import Path

from conveyor_engine.fact_predicates import (
    checks_completed,
    has_open_review_threads,
    is_capped,
    reset,
    review_run_skipped,
    review_run_succeeded,
    session_finished,
)
from conveyor_engine.github_client import PullRequestInfo, Subject
from conveyor_engine.guard_predicates import (
    all_checks_ran,
    all_prs_mergeable,
    all_prs_merged,
    has_access,
    has_open_prs,
    hotfix_pr_is_created,
    is_session_at_work,
    master_is_green,
    pr_is_mergeable,
    proposal_pr_is_mergeable,
)
from conveyor_engine.guards import _TOKEN_RE
from conveyor_engine.local_predicates import is_change_finished
from conveyor_engine.loader import Workflow, load_workflows

from .fakes import FakeClient

_KEYWORDS = {"AND", "OR", "NOT", "(", ")"}


def guard_strings(workflows: dict[str, Workflow]) -> list[str]:
    """Every non-empty `guard` string declared on any transition of any
    loaded workflow."""
    return [
        transition.guard
        for workflow in workflows.values()
        for state in workflow.states.values()
        for transition in state.transitions
        if transition.guard
    ]


def guard_names(workflows: dict[str, Workflow]) -> set[str]:
    """Every bare predicate name referenced anywhere in `guard_strings`,
    tokenized the same way `guards.evaluate_guard` itself does."""
    names: set[str] = set()
    for guard in guard_strings(workflows):
        for token in _TOKEN_RE.findall(guard):
            if token not in _KEYWORDS:
                names.add(token)
    return names


def _build_full_registry() -> dict[str, object]:
    """One real predicate (built from this section's own factories) per
    name -- never a placeholder lambda. A name with no entry here is a
    name this section did not implement."""
    repo = "o/r"
    issue = Subject(kind="issue", number=1, repo=repo)
    pr = Subject(kind="pull_request", number=2, repo=repo)
    client = FakeClient(
        permissions={(repo, "someone"): "write"},
        open_branches={repo: []},
        related={issue: [pr]},
        pr_infos={
            pr: PullRequestInfo(
                subject=pr,
                state="OPEN",
                merged=False,
                merge_state_status="CLEAN",
                checks_running=False,
            )
        },
        branch_conclusions={(repo, "master"): "success"},
    )
    return {
        "has_access": has_access(client, repo, "someone"),
        "is_session_at_work": is_session_at_work(client, repo, "change/x"),
        "has_open_prs": has_open_prs(client, issue),
        "all_prs_mergeable": all_prs_mergeable(client, issue),
        "all_prs_merged": all_prs_merged(client, issue),
        "master_is_green": master_is_green(client, repo),
        "pr_is_mergeable": pr_is_mergeable(client, pr),
        "proposal_pr_is_mergeable": proposal_pr_is_mergeable(client, pr),
        "hotfix_pr_is_created": hotfix_pr_is_created(client, pr),
        "all_checks_ran": all_checks_ran(client, pr),
        "is_change_finished": is_change_finished(Path(__file__)),
        "is_capped": is_capped(rounds_used=0, grants=0, max_rounds=5),
        "reset": reset(False),
        "session_finished": session_finished(True),
        "checks_completed": checks_completed(True),
        "review_run_succeeded": review_run_succeeded("success"),
        "has_open_review_threads": has_open_review_threads(0),
        "review_run_skipped": review_run_skipped(False),
    }


class GuardCoverageTest(unittest.TestCase):
    def test_every_guard_name_in_the_real_yaml_resolves(self):
        workflows = load_workflows()
        needed = guard_names(workflows)
        registry = _build_full_registry()

        missing = sorted(needed - registry.keys())
        self.assertEqual(
            missing,
            [],
            f"no predicate built for: {missing!r}",
        )

    def test_every_built_predicate_actually_calls(self):
        # Not vacuous: each entry really is a zero-argument callable that
        # resolves to a bool, not merely a key present in the dict.
        for name, predicate in _build_full_registry().items():
            with self.subTest(name=name):
                self.assertIn(predicate(), (True, False))


if __name__ == "__main__":
    unittest.main()
