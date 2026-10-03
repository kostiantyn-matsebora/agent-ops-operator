import contextlib
import io
import unittest

from conveyor_engine.github_client import Subject
from conveyor_engine.labels import LabelMapping, Prefix
from conveyor_engine.state_writer import StateWriter

from .fakes import FakeClient

ISSUE = Subject(kind="issue", number=1, repo="o/r")
PR = Subject(kind="pull_request", number=2, repo="o/r")
OTHER_PR = Subject(kind="pull_request", number=3, repo="o/r")


def _mapping(propagation: str) -> LabelMapping:
    return LabelMapping(
        prefixes={
            "station": Prefix(
                name="station",
                kind="state",
                native_subject="issue",
                propagation=propagation,
                set_manually_on=("issue",),
            )
        },
        triggers={},
        states={},
    )


VOCABULARY = {"station:ready", "station:implementing", "station:done"}


class WriteStateSuccessTest(unittest.TestCase):
    def test_label_set_after_successful_write(self):
        client = FakeClient(labels={ISSUE: {"station:ready", "priority:high"}})
        writer = StateWriter(_mapping("none"), client)

        wrote = writer.write_state(ISSUE, VOCABULARY, "station:implementing")

        self.assertTrue(wrote)
        self.assertEqual(
            client.get_labels(ISSUE), {"station:implementing", "priority:high"}
        )


class WriteStatePropagationTest(unittest.TestCase):
    def test_label_set_on_both_subjects_after_a_propagated_write(self):
        client = FakeClient(
            labels={ISSUE: {"station:ready"}, PR: {"review:needed"}},
            related={ISSUE: [PR]},
        )
        writer = StateWriter(_mapping("bidirectional"), client)

        wrote = writer.write_state(ISSUE, VOCABULARY, "station:implementing")

        self.assertTrue(wrote)
        self.assertEqual(client.get_labels(ISSUE), {"station:implementing"})
        self.assertEqual(
            client.get_labels(PR), {"review:needed", "station:implementing"}
        )

    def test_propagates_to_every_related_pull_request(self):
        client = FakeClient(
            labels={ISSUE: {"station:ready"}, PR: set(), OTHER_PR: set()},
            related={ISSUE: [PR, OTHER_PR]},
        )
        writer = StateWriter(_mapping("bidirectional"), client)

        writer.write_state(ISSUE, VOCABULARY, "station:implementing")

        self.assertIn("station:implementing", client.get_labels(PR))
        self.assertIn("station:implementing", client.get_labels(OTHER_PR))

    def test_non_propagating_prefix_stays_on_its_own_subject(self):
        client = FakeClient(
            labels={ISSUE: {"station:ready"}, PR: set()}, related={ISSUE: [PR]}
        )
        writer = StateWriter(_mapping("none"), client)

        writer.write_state(ISSUE, VOCABULARY, "station:implementing")

        self.assertEqual(client.get_labels(PR), set())

    def test_pull_request_to_issue_direction(self):
        client = FakeClient(
            labels={ISSUE: {"station:ready"}, PR: set()}, related={ISSUE: [PR]}
        )
        writer = StateWriter(_mapping("bidirectional"), client)

        # Propagation runs from whichever subject the label was WRITTEN on.
        # Writing a station label directly on the pull request must reach
        # back to its related issue.
        writer.write_state(PR, set(), "station:implementing")

        self.assertIn("station:implementing", client.get_labels(ISSUE))


class WriteStateFailureTest(unittest.TestCase):
    def test_a_simulated_api_failure_is_logged_and_swallowed(self):
        client = FakeClient(
            labels={ISSUE: {"station:ready"}}, fail_on="set_labels"
        )
        writer = StateWriter(_mapping("none"), client)

        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            wrote = writer.write_state(ISSUE, VOCABULARY, "station:implementing")

        self.assertFalse(wrote)
        self.assertIn("conveyor-engine", stderr.getvalue())
        # The write never raised past the caller.
        self.assertEqual(client.get_labels(ISSUE), {"station:ready"})

    def test_a_propagation_failure_does_not_undo_the_local_write(self):
        client = FakeClient(
            labels={ISSUE: {"station:ready"}, PR: set()},
            related={ISSUE: [PR]},
            fail_on="related_pull_requests",
        )
        writer = StateWriter(_mapping("bidirectional"), client)

        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            wrote = writer.write_state(ISSUE, VOCABULARY, "station:implementing")

        self.assertTrue(wrote)
        self.assertEqual(client.get_labels(ISSUE), {"station:implementing"})
        self.assertIn("conveyor-engine", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
