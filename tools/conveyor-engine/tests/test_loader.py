import unittest

from conveyor_engine.loader import (
    load_workflows,
    event_to_workflows,
    state_to_workflows,
    all_action_names,
    parse_workflows,
)

EXPECTED_WORKFLOW_NAMES = {
    "conveyor.propose",
    "conveyor.implement",
    "conveyor.fix",
    "conveyor.finalize",
    "loop",
    "conveyor.run",
    "review",
}


class LoadWorkflowsTest(unittest.TestCase):
    def test_every_declared_workflow_is_present(self):
        workflows = load_workflows()
        self.assertEqual(set(workflows.keys()), EXPECTED_WORKFLOW_NAMES)

    def test_conveyor_implement_shape(self):
        workflows = load_workflows()
        implement = workflows["conveyor.implement"]
        self.assertEqual(implement.subject, "issue")
        self.assertEqual(implement.label, "conveyor:implement")
        self.assertEqual(implement.initial_state(), "station:ready")
        self.assertIn("station:implementing", implement.states)

        ready = implement.states["station:ready"]
        self.assertTrue(ready.initial)
        self.assertEqual(len(ready.transitions), 1)
        transition = ready.transitions[0]
        self.assertEqual(transition.event, "conveyor:implement")
        self.assertEqual(transition.to, "station:implementing")
        self.assertEqual(transition.guard, "has_access AND NOT is_session_at_work")
        self.assertEqual(transition.owned_by, "fire")

    def test_review_is_standalone(self):
        workflows = load_workflows()
        review = workflows["review"]
        self.assertEqual(review.subject, "pull_request")
        self.assertEqual(review.acts_on, "scan")
        for state in review.states.values():
            self.assertIsNone(state.invokes)

    def test_invokes_is_captured(self):
        workflows = load_workflows()
        run = workflows["conveyor.run"]
        self.assertEqual(run.states["station:ready"].invokes, "conveyor.implement")


class ParseWorkflowsTest(unittest.TestCase):
    """Exercises the pure parser against a literal dict, with no file on
    disk -- the shape a hand-built fixture needs in the other test modules."""

    def test_minimal_workflow(self):
        raw = {
            "workflows": {
                "widget": {
                    "subject": "issue",
                    "acts_on": "station",
                    "states": {
                        "station:a": {
                            "initial": True,
                            "transitions": [
                                {"event": "go", "to": "station:b", "guard": "ok"}
                            ],
                        },
                        "station:b": {"final": True},
                    },
                }
            }
        }
        workflows = parse_workflows(raw)
        self.assertEqual(set(workflows.keys()), {"widget"})
        widget = workflows["widget"]
        self.assertEqual(widget.initial_state(), "station:a")
        self.assertTrue(widget.states["station:b"].final)
        self.assertEqual(widget.states["station:a"].transitions[0].guard, "ok")


class CrossReferenceHelpersTest(unittest.TestCase):
    def test_event_to_workflows_matches_real_file(self):
        workflows = load_workflows()
        mapping = event_to_workflows(workflows)
        self.assertEqual(mapping["conveyor:implement"], {"conveyor.implement"})
        self.assertEqual(mapping["loop:fix"], {"conveyor.fix", "loop"})
        self.assertEqual(
            mapping["all_prs_merged"],
            {"conveyor.implement", "conveyor.fix", "conveyor.finalize"},
        )

    def test_state_to_workflows_matches_real_file(self):
        workflows = load_workflows()
        mapping = state_to_workflows(workflows)
        self.assertEqual(
            mapping["station:ready"],
            {"conveyor.propose", "conveyor.implement", "conveyor.run"},
        )
        self.assertEqual(
            mapping["station:done"],
            {
                "conveyor.implement",
                "conveyor.fix",
                "conveyor.finalize",
                "conveyor.run",
            },
        )

    def test_all_action_names_matches_real_file(self):
        workflows = load_workflows()
        self.assertEqual(
            all_action_names(workflows),
            {"fire", "notify_findings", "notify_failure"},
        )


if __name__ == "__main__":
    unittest.main()
