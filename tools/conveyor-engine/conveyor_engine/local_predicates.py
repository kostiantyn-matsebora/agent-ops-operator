"""Guard predicate factories that read the LOCAL checkout, never GitHub.

`is_change_finished` is the one guard in this change whose fact lives in a
file (an openspec change's `tasks.md`) rather than behind `GitHubClient`.
Resolving WHICH change is bound to a given issue is left to the caller --
see this module's own docstring on `is_change_finished` for why.
"""
from __future__ import annotations

import re
from pathlib import Path
from typing import Callable

_CHECKBOX_RE = re.compile(r"^\s*-\s*\[([ xX])\]", re.MULTILINE)


def tasks_are_all_ticked(tasks_text: str) -> bool:
    """A change is finished when it has at least one task box and every one
    is ticked. An empty or box-less file is NOT finished -- the same rule
    this repository's own `conveyor.py:is_finished` states, re-derived
    rather than imported (see this package's own rule against importing
    `conveyor.py`)."""
    boxes = _CHECKBOX_RE.findall(tasks_text or "")
    return bool(boxes) and all(box in "xX" for box in boxes)


def is_change_finished(tasks_path: Path) -> Callable[[], bool]:
    """True when `tasks_path` exists, is readable, and every task box in it
    is ticked.

    `tasks_path` is resolved by the CALLER -- finding "the change bound to
    this issue" is a separate concern (in production, walking
    `openspec/changes/*/.github-issue` the way this repository's own
    `conveyor_io.py:bound_change()` does conceptually) that nothing calls
    this predicate for yet (see design.md's Non-Goals: no real caller is
    rewired in this change). A factory taking the path directly keeps this
    predicate's own job -- read one file, decide one boolean -- testable
    with no filesystem fixture beyond a temp file.

    FAILS CLOSED TOWARD "NOT FINISHED" (False) -- a missing or unreadable
    tasks file must never be read as "every task is done", which would let
    `conveyor.finalize` fire a session over a change that cannot say what
    it still owes.
    """

    def read() -> bool:
        try:
            text = tasks_path.read_text()
        except OSError:
            return False
        return tasks_are_all_ticked(text)

    return read
