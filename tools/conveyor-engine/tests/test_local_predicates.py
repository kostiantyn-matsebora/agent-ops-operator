import tempfile
import unittest
from pathlib import Path

from conveyor_engine.local_predicates import is_change_finished, tasks_are_all_ticked


class TasksAreAllTickedTest(unittest.TestCase):
    def test_every_box_ticked(self):
        text = "## 1. Section\n\n- [x] done\n- [X] also done\n"
        self.assertTrue(tasks_are_all_ticked(text))

    def test_one_box_unticked(self):
        text = "## 1. Section\n\n- [x] done\n- [ ] not done\n"
        self.assertFalse(tasks_are_all_ticked(text))

    def test_no_boxes_is_not_finished(self):
        self.assertFalse(tasks_are_all_ticked("# nothing here\n"))

    def test_empty_text_is_not_finished(self):
        self.assertFalse(tasks_are_all_ticked(""))


class IsChangeFinishedTest(unittest.TestCase):
    def test_finished(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "tasks.md"
            path.write_text("- [x] one\n- [x] two\n")
            self.assertTrue(is_change_finished(path)())

    def test_unfinished(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "tasks.md"
            path.write_text("- [x] one\n- [ ] two\n")
            self.assertFalse(is_change_finished(path)())

    def test_missing_file_fails_closed_to_not_finished(self):
        path = Path(tempfile.mkdtemp()) / "does-not-exist.md"
        self.assertFalse(is_change_finished(path)())


if __name__ == "__main__":
    unittest.main()
