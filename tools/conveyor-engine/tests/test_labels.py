import unittest

from conveyor_engine.labels import load_label_mapping, parse_label_mapping

EXPECTED_PREFIXES = {"conveyor", "station", "loop", "round", "scan"}


class LoadLabelMappingTest(unittest.TestCase):
    def test_every_prefix_is_present_and_complete(self):
        mapping = load_label_mapping()
        self.assertEqual(set(mapping.prefixes.keys()), EXPECTED_PREFIXES)
        for name, prefix in mapping.prefixes.items():
            self.assertEqual(prefix.name, name)
            self.assertIsInstance(prefix.kind, str)
            self.assertTrue(prefix.kind)
            self.assertIsInstance(prefix.native_subject, str)
            self.assertTrue(prefix.native_subject)
            self.assertIsInstance(prefix.propagation, str)
            self.assertTrue(prefix.propagation)
            self.assertIsInstance(prefix.set_manually_on, tuple)

    def test_station_is_bidirectional(self):
        mapping = load_label_mapping()
        self.assertEqual(mapping.prefixes["station"].propagation, "bidirectional")
        self.assertEqual(mapping.prefixes["loop"].propagation, "none")

    def test_every_trigger_names_real_workflow_strings(self):
        mapping = load_label_mapping()
        self.assertTrue(mapping.triggers)
        for name, trigger in mapping.triggers.items():
            self.assertEqual(trigger.name, name)
            self.assertIsInstance(trigger.workflows, tuple)
            self.assertTrue(trigger.workflows, f"{name} declares no owning workflow")
            for workflow_name in trigger.workflows:
                self.assertIsInstance(workflow_name, str)
                self.assertTrue(workflow_name)

    def test_every_state_names_real_workflow_strings(self):
        mapping = load_label_mapping()
        self.assertTrue(mapping.states)
        for name, state in mapping.states.items():
            self.assertEqual(state.name, name)
            self.assertIsInstance(state.workflows, tuple)
            self.assertTrue(state.workflows, f"{name} declares no owning workflow")
            for workflow_name in state.workflows:
                self.assertIsInstance(workflow_name, str)
                self.assertTrue(workflow_name)
            self.assertIsInstance(state.prefix, str)
            self.assertIn(state.prefix, mapping.prefixes)


class ParseLabelMappingTest(unittest.TestCase):
    def test_minimal_mapping(self):
        raw = {
            "prefixes": {
                "widget": {
                    "kind": "state",
                    "native_subject": "issue",
                    "propagation": "none",
                    "set_manually_on": ["issue"],
                }
            },
            "triggers": {
                "widget:go": {
                    "label": "widget:go",
                    "invocation": "label_placed",
                    "prefix": "widget",
                    "workflows": ["w"],
                }
            },
            "states": {
                "widget:a": {"workflows": ["w"], "prefix": "widget"},
            },
        }
        mapping = parse_label_mapping(raw)
        self.assertEqual(mapping.prefixes["widget"].propagation, "none")
        self.assertEqual(mapping.triggers["widget:go"].workflows, ("w",))
        self.assertEqual(mapping.states["widget:a"].prefix, "widget")


if __name__ == "__main__":
    unittest.main()
