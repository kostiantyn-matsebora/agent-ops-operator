"""Which openspec changes a diff range touches, and which it archives.

`pr-closes-guard.py` and `docs-task-guard.py` each walk `git diff --name-only
<range>` looking for paths under `openspec/changes/archive/<YYYY-MM-DD-name>/`
-- the guards' shared fact that a change was just archived by this diff. Only
the SHAPE of what each wanted back differed (a bare name, or a name mapped to
its new tasks.md path), so that stays the caller's own small wrapper.
"""
from __future__ import annotations

import pathlib
import re
import subprocess

ARCHIVED_DIR = re.compile(r"^\d{4}-\d{2}-\d{2}-(.+)$")


def diff_paths(diff_range: str, root: pathlib.Path) -> list[str]:
    """Every path this diff range touches. Raises (OSError,
    subprocess.CalledProcessError) on a failed read -- callers disagree on
    what an unreadable diff should mean (one is silent, one prints why), so
    that stays theirs to decide rather than a policy baked in here."""
    out = subprocess.run(
        ["git", "diff", "--name-only", diff_range],
        cwd=root, capture_output=True, text=True, check=True,
    ).stdout
    return out.splitlines()


def archived_change_names(diff_range: str, root: pathlib.Path) -> dict[str, str]:
    """change name -> its archived directory name (`YYYY-MM-DD-<name>`), for
    every change this diff moves into `openspec/changes/archive/`. `{}` on an
    unreadable diff -- the archive question is always safe to answer "none"."""
    try:
        paths = diff_paths(diff_range, root)
    except (OSError, subprocess.CalledProcessError):
        return {}
    found: dict[str, str] = {}
    for path in paths:
        parts = path.split("/")
        if len(parts) > 4 and parts[:3] == ["openspec", "changes", "archive"]:
            if m := ARCHIVED_DIR.match(parts[3]):
                found[m.group(1)] = parts[3]
    return found
