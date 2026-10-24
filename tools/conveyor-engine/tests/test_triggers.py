import unittest

from conveyor_engine.loader import State, Transition, Workflow
from conveyor_engine.triggers import match_transitions


def _workflow(states: dict[str, State]) -> Workflow:
    return Workflow(name="widget", subject="issue", acts_on="station", states=states)


class MatchTransitionsTest(unittest.TestCase):
    def test_a_match(self):
        workflow = _workflow(
            {
                "s1": State(
                    id="s1",
                    transitions=(Transition(event="go", to="s2"),),
                )
            }
        )
        result = match_transitions(workflow, "s1", "go")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0].to, "s2")

    def test_a_non_match(self):
        workflow = _workflow(
            {
                "s1": State(
                    id="s1",
                    transitions=(Transition(event="go", to="s2"),),
                )
            }
        )
        self.assertEqual(match_transitions(workflow, "s1", "other_event"), [])

    def test_unknown_state(self):
        workflow = _workflow({"s1": State(id="s1", transitions=())})
        self.assertEqual(match_transitions(workflow, "nowhere", "go"), [])

    def test_two_transitions_same_event_different_guards(self):
        t_true = Transition(event="go", to="s2", guard="guard_a")
        t_false = Transition(event="go", to="s3", guard="guard_b")
        workflow = _workflow(
            {"s1": State(id="s1", transitions=(t_true, t_false))}
        )
        result = match_transitions(workflow, "s1", "go")
        self.assertEqual(result, [t_true, t_false])


if __name__ == "__main__":
    unittest.main()
