"""Asserts `workflows.desired.yaml` and `labels.yaml` agree with each
other: every event and every state the workflow file declares has exactly
one matching entry in the label mapping, and that entry's declared owning
workflow(s) match the workflow file exactly -- in both directions, so an
orphaned mapping entry is caught too.
"""
import unittest

from conveyor_engine.labels import load_label_mapping
from conveyor_engine.loader import event_to_workflows, load_workflows, state_to_workflows


class WorkflowLabelConsistencyTest(unittest.TestCase):
    def setUp(self):
        self.workflows = load_workflows()
        self.label_mapping = load_label_mapping()
        self.events = event_to_workflows(self.workflows)
        self.states = state_to_workflows(self.workflows)

    def test_every_declared_event_has_exactly_one_matching_trigger_entry(self):
        missing = sorted(set(self.events) - set(self.label_mapping.triggers))
        self.assertEqual(
            missing,
            [],
            f"events declared in workflows.desired.yaml with no entry in "
            f"labels.yaml's triggers: {missing}",
        )

    def test_every_trigger_entry_names_a_real_declared_event(self):
        extra = sorted(set(self.label_mapping.triggers) - set(self.events))
        self.assertEqual(
            extra,
            [],
            f"labels.yaml triggers naming an event no workflow declares: {extra}",
        )

    def test_every_trigger_entrys_owning_workflows_match_exactly(self):
        mismatches = []
        for event, declared_workflows in self.events.items():
            trigger = self.label_mapping.triggers.get(event)
            if trigger is None:
                continue  # reported by the earlier test
            mapped_workflows = set(trigger.workflows)
            if mapped_workflows != declared_workflows:
                mismatches.append(
                    f"{event}: workflows.desired.yaml declares "
                    f"{sorted(declared_workflows)}, labels.yaml declares "
                    f"{sorted(mapped_workflows)}"
                )
        self.assertEqual(mismatches, [], "\n".join(mismatches))

    def test_every_declared_state_has_exactly_one_matching_state_entry(self):
        missing = sorted(set(self.states) - set(self.label_mapping.states))
        self.assertEqual(
            missing,
            [],
            f"states declared in workflows.desired.yaml with no entry in "
            f"labels.yaml's states: {missing}",
        )

    def test_every_state_entry_names_a_real_declared_state(self):
        extra = sorted(set(self.label_mapping.states) - set(self.states))
        self.assertEqual(
            extra,
            [],
            f"labels.yaml states naming a state no workflow declares: {extra}",
        )

    def test_every_state_entrys_owning_workflows_match_exactly(self):
        mismatches = []
        for state_id, declared_workflows in self.states.items():
            mapped = self.label_mapping.states.get(state_id)
            if mapped is None:
                continue  # reported by the earlier test
            mapped_workflows = set(mapped.workflows)
            if mapped_workflows != declared_workflows:
                mismatches.append(
                    f"{state_id}: workflows.desired.yaml declares "
                    f"{sorted(declared_workflows)}, labels.yaml declares "
                    f"{sorted(mapped_workflows)}"
                )
        self.assertEqual(mismatches, [], "\n".join(mismatches))

    def test_every_state_entry_names_a_real_prefix(self):
        unknown = sorted(
            {
                f"{name} -> {state.prefix}"
                for name, state in self.label_mapping.states.items()
                if state.prefix not in self.label_mapping.prefixes
            }
        )
        self.assertEqual(unknown, [], f"state entries naming an undeclared prefix: {unknown}")


if __name__ == "__main__":
    unittest.main()
