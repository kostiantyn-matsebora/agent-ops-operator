#!/usr/bin/env python3
"""Every openspec change ends with a COMPLETED documentation section.

`.claude/rules/documentation.md` states the rule and says it is enforced rather
than trusted. It was enforced in exactly one place: a PreToolUse hook in one
contributor's local harness, refusing `openspec archive`. That is absent for
every other contributor, for every session whose harness is not installed, and
for automation -- and this repository is public now, so "every other
contributor" is a real population.

THIS IS THE ONE IMPLEMENTATION OF THE DECISION. The hook calls it too, so the
local gate and the CI check cannot drift into two answers to one question. The
hook keeps its own job -- deciding WHEN to ask, and failing open when it cannot
-- but it no longer decides WHAT the answer is.

WHAT IS CHECKED, and both halves were paid for:

  1. the LAST `## ` section of tasks.md is a documentation section
  2. no task under it is unticked

WHY ONLY THE CHANGES A PULL REQUEST TOUCHED. Eleven changes are in flight at any
time and most have unticked documentation tasks, correctly -- they are not
finished. A guard that judged all of them would fail every pull request for work
it was not about, and would be switched off within a day.
"""
from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys

HEADING = re.compile(r"^## +(.*)$")
UNTICKED = re.compile(r"^\s*- \[ \]")
DOCUMENTATION = re.compile(r"documentation", re.I)

BOTH_HALVES = """Every change ends with a dedicated documentation section covering BOTH halves,
listed separately because they are skipped independently:

  1. the reference docs   - docs/concepts.md, docs/contracts.md, a bundle page,
                            docs/CHANGELOG.md
  2. THE ADOPTER SITE     - the landing page, introduction.md,
                            getting-started.md, installation.md, docs/guides/*"""


def verdict(tasks: pathlib.Path) -> tuple[bool, str]:
    """(ok, message). A file that cannot be read is OK -- see module docstring."""
    try:
        lines = tasks.read_text(encoding="utf-8").splitlines()
    except OSError:
        return True, ""

    headings = [(i, m.group(1).strip()) for i, l in enumerate(lines) if (m := HEADING.match(l))]
    if not headings:
        return True, ""

    index, title = headings[-1]
    if not DOCUMENTATION.search(title):
        return False, (
            f"{tasks}: the LAST section is not a documentation section.\n\n"
            f"  its final section is: {title}\n\n{BOTH_HALVES}"
        )

    outstanding = [l.strip() for l in lines[index:] if UNTICKED.match(l)]
    if outstanding:
        listed = "\n".join(f"  {l}" for l in outstanding)
        return False, (
            f"{tasks}: {len(outstanding)} unfinished documentation task(s).\n\n{listed}\n\n"
            "Reporting the change finished while the half a reader meets is not."
        )
    return True, ""


def changed_changes(diff_range: str, root: pathlib.Path) -> list[str]:
    """Change names touched by a diff range, archived ones excluded."""
    try:
        out = subprocess.run(
            ["git", "diff", "--name-only", diff_range],
            cwd=root, capture_output=True, text=True, check=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"docs-task-guard: cannot read the diff range ({exc}); nothing to check",
              file=sys.stderr)
        return []

    names = set()
    for path in out.splitlines():
        parts = path.split("/")
        # openspec/changes/<name>/... , but never openspec/changes/archive/...
        if len(parts) > 3 and parts[:2] == ["openspec", "changes"] and parts[2] != "archive":
            names.add(parts[2])
    return sorted(names)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--tasks", type=pathlib.Path,
                    help="judge one tasks file (the hook's mode)")
    ap.add_argument("--range", dest="diff_range",
                    help="judge every change touched by a git diff range (CI's mode)")
    ap.add_argument("--root", type=pathlib.Path, default=pathlib.Path("."),
                    help="repository root")
    args = ap.parse_args()

    if args.tasks:
        ok, message = verdict(args.tasks)
        if not ok:
            print(message, file=sys.stderr)
        return 0 if ok else 1

    if not args.diff_range:
        ap.error("one of --tasks or --range is required")

    names = changed_changes(args.diff_range, args.root)
    if not names:
        print("docs-task-guard: this pull request touches no openspec change")
        return 0

    failed = 0
    for name in names:
        tasks = args.root / "openspec" / "changes" / name / "tasks.md"
        if not tasks.is_file():
            # A change being DELETED by this diff, or one with no task list yet.
            print(f"  skipped  {name} (no tasks file)")
            continue
        ok, message = verdict(tasks)
        if ok:
            print(f"  ok       {name}")
        else:
            failed += 1
            print(f"  FAILED   {name}")
            print(message, file=sys.stderr)

    if failed:
        print(f"\n{failed} change(s) report finished work whose documentation is not.",
              file=sys.stderr)
        print("See .claude/rules/documentation.md.", file=sys.stderr)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
