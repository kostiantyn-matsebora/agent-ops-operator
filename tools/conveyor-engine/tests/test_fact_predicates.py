import dataclasses
import unittest

from conveyor_engine.fact_predicates import (
    cap_for,
    checks_completed,
    count_marker_occurrences,
    has_open_review_threads,
    is_capped,
    reset,
    review_run_skipped,
    review_run_succeeded,
    session_finished,
)


@dataclasses.dataclass
class _Comment:
    created_at: str
    body: str


class CapForTest(unittest.TestCase):
    def test_no_grants(self):
        self.assertEqual(cap_for(max_rounds=5, grants=0), 5)

    def test_one_grant_adds_a_full_set(self):
        self.assertEqual(cap_for(max_rounds=5, grants=1), 10)

    def test_two_grants(self):
        self.assertEqual(cap_for(max_rounds=5, grants=2), 15)


class CountMarkerOccurrencesTest(unittest.TestCase):
    def test_counts_a_standalone_marker_line(self):
        comments = [_Comment("2026-01-01T00:00:00Z", "<!-- marker -->\nbody")]
        self.assertEqual(count_marker_occurrences(comments, "<!-- marker -->"), 1)

    def test_does_not_count_a_mere_substring_mention(self):
        comments = [_Comment("2026-01-01T00:00:00Z", "I quoted `<!-- marker -->` above")]
        self.assertEqual(count_marker_occurrences(comments, "<!-- marker -->"), 0)

    def test_since_excludes_earlier_comments(self):
        comments = [
            _Comment("2026-01-01T00:00:00Z", "<!-- marker -->"),
            _Comment("2026-02-01T00:00:00Z", "<!-- marker -->"),
        ]
        self.assertEqual(
            count_marker_occurrences(comments, "<!-- marker -->", since="2026-01-15T00:00:00Z"),
            1,
        )


class IsCappedTest(unittest.TestCase):
    def test_under_the_cap(self):
        self.assertFalse(is_capped(rounds_used=2, grants=0, max_rounds=5)())

    def test_at_the_cap(self):
        self.assertTrue(is_capped(rounds_used=5, grants=0, max_rounds=5)())

    def test_a_grant_raises_the_cap(self):
        self.assertFalse(is_capped(rounds_used=7, grants=1, max_rounds=5)())
        self.assertTrue(is_capped(rounds_used=10, grants=1, max_rounds=5)())

    def test_unreadable_fails_closed_to_already_capped(self):
        self.assertTrue(is_capped(rounds_used=None, grants=0, max_rounds=5)())


class ResetTest(unittest.TestCase):
    def test_granted(self):
        self.assertTrue(reset(True)())

    def test_not_granted(self):
        self.assertFalse(reset(False)())

    def test_unreadable_fails_closed_to_not_granted(self):
        self.assertFalse(reset(None)())


class SessionFinishedTest(unittest.TestCase):
    def test_finished(self):
        self.assertTrue(session_finished(True)())

    def test_not_finished(self):
        self.assertFalse(session_finished(False)())

    def test_unreadable_fails_closed_to_not_finished(self):
        self.assertFalse(session_finished(None)())


class ChecksCompletedTest(unittest.TestCase):
    def test_completed(self):
        self.assertTrue(checks_completed(True)())

    def test_not_completed(self):
        self.assertFalse(checks_completed(False)())

    def test_unreadable_fails_closed_to_not_completed(self):
        self.assertFalse(checks_completed(None)())


class ReviewRunSucceededTest(unittest.TestCase):
    def test_success(self):
        self.assertTrue(review_run_succeeded("success")())

    def test_failure(self):
        self.assertFalse(review_run_succeeded("failure")())

    def test_unreadable_fails_closed_to_not_landed(self):
        self.assertFalse(review_run_succeeded(None)())


class HasOpenReviewThreadsTest(unittest.TestCase):
    def test_true_when_threads_open(self):
        self.assertTrue(has_open_review_threads(3)())

    def test_false_when_zero(self):
        self.assertFalse(has_open_review_threads(0)())

    def test_unreadable_fails_closed_to_threads_open(self):
        self.assertTrue(has_open_review_threads(None)())


class ReviewRunSkippedTest(unittest.TestCase):
    def test_skipped(self):
        self.assertTrue(review_run_skipped(True)())

    def test_not_skipped(self):
        self.assertFalse(review_run_skipped(False)())

    def test_unreadable_fails_closed_to_not_skipped(self):
        self.assertFalse(review_run_skipped(None)())


if __name__ == "__main__":
    unittest.main()
