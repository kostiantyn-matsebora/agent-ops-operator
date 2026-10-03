import contextlib
import io
import unittest

from conveyor_engine.actions import build_action_registry
from conveyor_engine.engine import Engine
from conveyor_engine.github_client import Subject
from conveyor_engine.labels import load_label_mapping
from conveyor_engine.loader import load_workflows

from .fakes import FakeClient

ISSUE = Subject(kind="issue", number=100, repo="o/r")


class EngineEvaluateTest(unittest.TestCase):
    """Drives a full transition end to end against `conveyor.implement`'s
    REAL declared shape, loaded from the real YAML files."""

    def setUp(self):
        self.workflows = load_workflows()
        self.label_mapping = load_label_mapping()
        self.action_registry = build_action_registry(self.workflows)
        self.client = FakeClient(labels={ISSUE: {"station:ready"}})
        self.guard_registry = {
            "has_access": lambda: True,
            "is_session_at_work": lambda: False,
        }
        self.engine = Engine(
            workflows=self.workflows,
            label_mapping=self.label_mapping,
            guard_registry=self.guard_registry,
            action_registry=self.action_registry,
            client=self.client,
        )

    def test_satisfied_guard_moves_state_and_calls_the_stub(self):
        stdout = io.StringIO()
        with contextlib.redirect_stdout(stdout):
            result = self.engine.evaluate(
                "conveyor.implement",
                ISSUE,
                "conveyor:implement",
                facts={"actor": "komkom"},
            )

        self.assertTrue(result.moved)
        self.assertEqual(result.from_state, "station:ready")
        self.assertEqual(result.to_state, "station:implementing")
        self.assertEqual(result.action_called, "fire")
        self.assertEqual(self.client.get_labels(ISSUE), {"station:implementing"})
        self.assertIn("action=fire", stdout.getvalue())

    def test_unsatisfied_guard_moves_nothing(self):
        self.guard_registry["is_session_at_work"] = lambda: True  # already at work

        result = self.engine.evaluate("conveyor.implement", ISSUE, "conveyor:implement")

        self.assertFalse(result.moved)
        self.assertEqual(self.client.get_labels(ISSUE), {"station:ready"})

    def test_unrecognized_event_moves_nothing(self):
        result = self.engine.evaluate("conveyor.implement", ISSUE, "not_a_real_event")

        self.assertFalse(result.moved)
        self.assertEqual(result.from_state, "station:ready")
        self.assertEqual(self.client.get_labels(ISSUE), {"station:ready"})

    def test_two_satisfied_guards_on_one_event_move_nothing_and_log(self):
        # review's scan:running has FOUR transitions on "review:run_completed".
        # review_run_skipped is independent of the other two guards, so a
        # (contrived) combination makes two of the four transitions true at
        # once -- exactly the case the engine must refuse to pick between.
        pr = Subject(kind="pull_request", number=200, repo="o/r")
        client = FakeClient(labels={pr: {"scan:running"}})
        guard_registry = {
            "review_run_succeeded": lambda: True,
            "has_open_review_threads": lambda: False,
            "review_run_skipped": lambda: True,
        }
        engine = Engine(
            workflows=self.workflows,
            label_mapping=self.label_mapping,
            guard_registry=guard_registry,
            action_registry=self.action_registry,
            client=client,
        )

        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            result = engine.evaluate("review", pr, "review:run_completed")

        self.assertFalse(result.moved)
        self.assertIsNotNone(result.error)
        self.assertIn("scan:clean", result.error)
        self.assertIn("scan:skipped", result.error)
        self.assertIn("conveyor-engine", stderr.getvalue())
        self.assertEqual(client.get_labels(pr), {"scan:running"})

    def test_action_is_called_after_the_state_write_not_before(self):
        # A stub that merely runs is not proof of ORDER: it has to read live
        # state at call time and see whether the new label already landed.
        seen_labels_at_call_time = []

        def spy_action(workflow_name, subject, facts):
            seen_labels_at_call_time.append(set(self.client.get_labels(subject)))

        self.action_registry["fire"] = spy_action

        result = self.engine.evaluate(
            "conveyor.implement", ISSUE, "conveyor:implement"
        )

        self.assertTrue(result.moved)
        self.assertEqual(len(seen_labels_at_call_time), 1)
        self.assertEqual(seen_labels_at_call_time[0], {"station:implementing"})

    def test_unknown_workflow_name(self):
        result = self.engine.evaluate("no.such.workflow", ISSUE, "anything")
        self.assertFalse(result.moved)
        self.assertIsNotNone(result.error)

    def test_invoking_transition_fires_on_a_sub_workflow_entry_event(self):
        # conveyor.propose's station:proposing declares `invokes: loop` and a
        # transition on "loop_entered_mergeable" -- the SAME generic
        # mechanism as any other event: nothing special exists in the engine
        # for `invokes`, which is exactly what the spec requires ("the
        # invoking workflow's own transitions fire on events naming the
        # invoked workflow's state entries, not on the invoked workflow's
        # internal transitions").
        issue = Subject(kind="issue", number=300, repo="o/r")
        client = FakeClient(labels={issue: {"station:proposing"}})
        engine = Engine(
            workflows=self.workflows,
            label_mapping=self.label_mapping,
            guard_registry={"proposal_pr_is_mergeable": lambda: True},
            action_registry=self.action_registry,
            client=client,
        )

        result = engine.evaluate("conveyor.propose", issue, "loop_entered_mergeable")

        self.assertTrue(result.moved)
        self.assertEqual(result.to_state, "station:proposed")

    def test_invoked_workflows_internal_transitions_are_invisible_to_the_parent(self):
        # The invoked workflow (`loop`) really does move on its own event,
        # evaluated on its OWN subject -- but sending that same internal
        # event name to the PARENT's subject moves nothing there, because
        # `conveyor.propose` declares no transition for it. Nothing
        # propagates automatically: a caller choosing to invoke `loop` is
        # responsible for watching its state and firing the matching
        # `*_entered_*` event on the parent itself (`labels.yaml`'s own
        # `sub_workflow_entry` docstring: "observed by the invoking
        # workflow").
        pr = Subject(kind="pull_request", number=400, repo="o/r")
        loop_engine = Engine(
            workflows=self.workflows,
            label_mapping=self.label_mapping,
            guard_registry={"has_access": lambda: True},
            action_registry=self.action_registry,
            client=FakeClient(labels={pr: {"round:started"}}),
        )
        loop_result = loop_engine.evaluate("loop", pr, "loop:fix")
        self.assertTrue(loop_result.moved)
        self.assertEqual(loop_result.to_state, "round:fixing")

        issue = Subject(kind="issue", number=301, repo="o/r")
        parent_client = FakeClient(labels={issue: {"station:proposing"}})
        parent_engine = Engine(
            workflows=self.workflows,
            label_mapping=self.label_mapping,
            guard_registry={},
            action_registry=self.action_registry,
            client=parent_client,
        )

        parent_result = parent_engine.evaluate("conveyor.propose", issue, "loop:fix")

        self.assertFalse(parent_result.moved)
        self.assertEqual(parent_client.get_labels(issue), {"station:proposing"})

    def test_subject_with_no_recognizable_state(self):
        blank = Subject(kind="issue", number=999, repo="o/r")
        client = FakeClient(labels={blank: {"size:L"}})
        engine = Engine(
            workflows=self.workflows,
            label_mapping=self.label_mapping,
            guard_registry=self.guard_registry,
            action_registry=self.action_registry,
            client=client,
        )
        result = engine.evaluate("conveyor.implement", blank, "conveyor:implement")
        self.assertFalse(result.moved)
        self.assertIsNotNone(result.error)


if __name__ == "__main__":
    unittest.main()
