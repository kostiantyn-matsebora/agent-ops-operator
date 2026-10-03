import unittest

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

from .fakes import FakeClient

REPO = "o/r"
ISSUE = Subject(kind="issue", number=1, repo=REPO)
PR1 = Subject(kind="pull_request", number=11, repo=REPO)
PR2 = Subject(kind="pull_request", number=12, repo=REPO)


def _pr_info(subject, *, state="OPEN", merged=False, merge_state_status="", checks_running=False):
    return PullRequestInfo(
        subject=subject,
        state=state,
        merged=merged,
        merge_state_status=merge_state_status,
        checks_running=checks_running,
    )


class HasAccessTest(unittest.TestCase):
    def test_granted(self):
        client = FakeClient(permissions={(REPO, "komkom"): "write"})
        self.assertTrue(has_access(client, REPO, "komkom")())

    def test_denied(self):
        client = FakeClient(permissions={(REPO, "stranger"): "read"})
        self.assertFalse(has_access(client, REPO, "stranger")())

    def test_unreadable_fails_closed_to_no_access(self):
        client = FakeClient(fail_on="permission")
        self.assertFalse(has_access(client, REPO, "komkom")())


class IsSessionAtWorkTest(unittest.TestCase):
    def test_open_from_the_branch(self):
        client = FakeClient(open_branches={REPO: ["change/x"]})
        self.assertTrue(is_session_at_work(client, REPO, "change/x")())

    def test_nothing_open_from_the_branch(self):
        client = FakeClient(open_branches={REPO: ["change/other"]})
        self.assertFalse(is_session_at_work(client, REPO, "change/x")())

    def test_unreadable_fails_closed_to_already_at_work(self):
        client = FakeClient(fail_on="open_pull_request_branches")
        self.assertTrue(is_session_at_work(client, REPO, "change/x")())


class HasOpenPrsTest(unittest.TestCase):
    def test_true_when_one_related_pr_is_open(self):
        client = FakeClient(
            related={ISSUE: [PR1]},
            pr_infos={PR1: _pr_info(PR1, state="OPEN")},
        )
        self.assertTrue(has_open_prs(client, ISSUE)())

    def test_false_when_every_related_pr_is_closed(self):
        client = FakeClient(
            related={ISSUE: [PR1]},
            pr_infos={PR1: _pr_info(PR1, state="MERGED")},
        )
        self.assertFalse(has_open_prs(client, ISSUE)())

    def test_unreadable_fails_closed_to_not_yet(self):
        client = FakeClient(fail_on="related_pull_requests")
        self.assertFalse(has_open_prs(client, ISSUE)())


class AllPrsMergeableTest(unittest.TestCase):
    def test_true_when_every_related_pr_is_clean(self):
        client = FakeClient(
            related={ISSUE: [PR1, PR2]},
            pr_infos={
                PR1: _pr_info(PR1, merge_state_status="CLEAN"),
                PR2: _pr_info(PR2, merge_state_status="CLEAN"),
            },
        )
        self.assertTrue(all_prs_mergeable(client, ISSUE)())

    def test_false_when_one_related_pr_is_blocked(self):
        client = FakeClient(
            related={ISSUE: [PR1, PR2]},
            pr_infos={
                PR1: _pr_info(PR1, merge_state_status="CLEAN"),
                PR2: _pr_info(PR2, merge_state_status="BLOCKED"),
            },
        )
        self.assertFalse(all_prs_mergeable(client, ISSUE)())

    def test_false_when_no_related_pr_exists(self):
        client = FakeClient()
        self.assertFalse(all_prs_mergeable(client, ISSUE)())

    def test_unreadable_fails_closed_to_not_yet(self):
        client = FakeClient(fail_on="related_pull_requests")
        self.assertFalse(all_prs_mergeable(client, ISSUE)())


class PrIsMergeableTest(unittest.TestCase):
    def test_true_when_clean(self):
        client = FakeClient(pr_infos={PR1: _pr_info(PR1, merge_state_status="CLEAN")})
        self.assertTrue(pr_is_mergeable(client, PR1)())

    def test_false_when_blocked(self):
        client = FakeClient(pr_infos={PR1: _pr_info(PR1, merge_state_status="BLOCKED")})
        self.assertFalse(pr_is_mergeable(client, PR1)())

    def test_unreadable_fails_closed_to_not_yet(self):
        client = FakeClient(fail_on="pull_request_info")
        self.assertFalse(pr_is_mergeable(client, PR1)())

    def test_proposal_pr_is_mergeable_is_the_same_read(self):
        client = FakeClient(pr_infos={PR1: _pr_info(PR1, merge_state_status="CLEAN")})
        self.assertTrue(proposal_pr_is_mergeable(client, PR1)())


class AllPrsMergedTest(unittest.TestCase):
    def test_true_when_every_related_pr_merged(self):
        client = FakeClient(
            related={ISSUE: [PR1, PR2]},
            pr_infos={
                PR1: _pr_info(PR1, state="MERGED", merged=True),
                PR2: _pr_info(PR2, state="MERGED", merged=True),
            },
        )
        self.assertTrue(all_prs_merged(client, ISSUE)())

    def test_false_when_one_related_pr_unmerged(self):
        client = FakeClient(
            related={ISSUE: [PR1, PR2]},
            pr_infos={
                PR1: _pr_info(PR1, state="MERGED", merged=True),
                PR2: _pr_info(PR2, state="OPEN", merged=False),
            },
        )
        self.assertFalse(all_prs_merged(client, ISSUE)())

    def test_false_when_no_related_pr_exists(self):
        client = FakeClient()
        self.assertFalse(all_prs_merged(client, ISSUE)())

    def test_unreadable_fails_closed_to_not_yet(self):
        client = FakeClient(fail_on="related_pull_requests")
        self.assertFalse(all_prs_merged(client, ISSUE)())


class MasterIsGreenTest(unittest.TestCase):
    def test_true_on_success(self):
        client = FakeClient(branch_conclusions={(REPO, "master"): "success"})
        self.assertTrue(master_is_green(client, REPO)())

    def test_false_on_failure(self):
        client = FakeClient(branch_conclusions={(REPO, "master"): "failure"})
        self.assertFalse(master_is_green(client, REPO)())

    def test_unreadable_fails_closed_to_not_yet(self):
        client = FakeClient(fail_on="branch_check_conclusion")
        self.assertFalse(master_is_green(client, REPO)())


class HotfixPrIsCreatedTest(unittest.TestCase):
    def test_true_when_open(self):
        client = FakeClient(pr_infos={PR1: _pr_info(PR1, state="OPEN")})
        self.assertTrue(hotfix_pr_is_created(client, PR1)())

    def test_true_when_merged(self):
        client = FakeClient(pr_infos={PR1: _pr_info(PR1, state="MERGED", merged=True)})
        self.assertTrue(hotfix_pr_is_created(client, PR1)())

    def test_unreadable_fails_closed_to_already_created(self):
        client = FakeClient(fail_on="pull_request_info")
        self.assertTrue(hotfix_pr_is_created(client, PR1)())


class AllChecksRanTest(unittest.TestCase):
    def test_true_when_nothing_is_running(self):
        client = FakeClient(pr_infos={PR1: _pr_info(PR1, checks_running=False)})
        self.assertTrue(all_checks_ran(client, PR1)())

    def test_false_when_a_check_is_running(self):
        client = FakeClient(pr_infos={PR1: _pr_info(PR1, checks_running=True)})
        self.assertFalse(all_checks_ran(client, PR1)())

    def test_unreadable_fails_closed_to_not_yet(self):
        client = FakeClient(fail_on="pull_request_info")
        self.assertFalse(all_checks_ran(client, PR1)())


if __name__ == "__main__":
    unittest.main()
