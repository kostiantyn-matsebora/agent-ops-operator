"""Repository-root discovery shared by every loader in this package.

Each loader accepts an explicit `repo_root`, but the default lets the
package be invoked from anywhere: `default_repo_root` walks up from a
starting file (this module's own location, unless told otherwise) until it
finds a directory containing `.github/conveyor-model/workflows.desired.yaml`
-- this repository's one fixed landmark for the conveyor's data files, and
the only fact this module hardcodes.
"""
from __future__ import annotations

from pathlib import Path
from typing import Optional

_LANDMARK = Path(".github/conveyor-model/workflows.desired.yaml")
_MAX_LEVELS = 20


def default_repo_root(start: Optional[Path] = None) -> Path:
    """Walk upward from `start` (default: this file) to the first ancestor
    directory containing `.github/conveyor-model/workflows.desired.yaml`.

    Raises FileNotFoundError if no such ancestor exists within
    `_MAX_LEVELS` levels -- fail loud rather than guess a wrong root.
    """
    current = (start or Path(__file__)).resolve()
    if current.is_file():
        current = current.parent

    for _ in range(_MAX_LEVELS):
        if (current / _LANDMARK).is_file():
            return current
        parent = current.parent
        if parent == current:
            break
        current = parent

    raise FileNotFoundError(
        f"could not find {_LANDMARK} walking up from {start or Path(__file__)}"
    )
