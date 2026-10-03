import contextlib
import io
import unittest

from conveyor_engine.actions import build_action_registry
from conveyor_engine.github_client import Subject
from conveyor_engine.loader import load_workflows

from .fakes import RecordingClient


class BuildActionRegistryTest(unittest.TestCase):
    def test_every_referenced_action_has_a_stub(self):
        workflows = load_workflows()
        registry = build_action_registry(workflows)
        self.assertEqual(set(registry.keys()), {"fire", "notify_findings", "notify_failure"})

    def test_calling_a_stub_never_touches_a_client(self):
        workflows = load_workflows()
        recorder = RecordingClient()
        registry = build_action_registry(workflows, client=recorder)

        subject = Subject(kind="issue", number=1)
        stdout = io.StringIO()
        with contextlib.redirect_stdout(stdout):
            for name, stub in registry.items():
                stub("conveyor.implement", subject, {"actor": "someone"})

        self.assertEqual(recorder.calls, [])
        # It did record SOMETHING, so the assertion above is not vacuous.
        self.assertIn("::notice::", stdout.getvalue())
        self.assertIn("action=fire", stdout.getvalue())

    def test_stub_logs_the_facts_it_was_called_with(self):
        workflows = load_workflows()
        registry = build_action_registry(workflows)
        subject = Subject(kind="issue", number=42, repo="o/r")

        stdout = io.StringIO()
        with contextlib.redirect_stdout(stdout):
            registry["fire"]("conveyor.implement", subject, {"actor": "komkom"})

        output = stdout.getvalue()
        self.assertIn("action=fire", output)
        self.assertIn("workflow=conveyor.implement", output)
        self.assertIn("issue#42", output)
        self.assertIn("komkom", output)


if __name__ == "__main__":
    unittest.main()
