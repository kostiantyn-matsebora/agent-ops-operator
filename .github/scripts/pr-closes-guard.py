#!/usr/bin/env python3
"""A change's tracking issue closes at ARCHIVE, never at merge.

`change-issue-tracking` says every change has exactly one issue, "opened when the
change is proposed and closed when the change is archived". GitHub closes an
issue by itself when a merged pull request's body says `Closes #<n>` — so a
PROPOSAL that fills in the template's closing keyword ends the issue that was
meant to follow the change through applying, review and archiving.

FOUND ON A REAL PULL REQUEST. #54 proposes `coordinated-agents` and says
`Closes #53`; merging it would have closed that change's tracking issue with the
change not yet started, and nothing would have said so.

THE TEMPLATE NOW SAYS `Refs #`, AND THIS IS WHY THAT IS NOT ENOUGH. A template is
a suggestion at the moment somebody is typing quickly, and the failure is silent:
the issue is closed by GitHub, days later, in a thread nobody is reading.

AND THE OTHER WAY ROUND: a pull request that ARCHIVES a change MUST carry
`Closes #<n>` for that change's issue, or the guard refuses it — two archived
changes sat with open issues because the keyword was simply not written.

WHAT IS ALLOWED. `Closes #<n>` is correct in exactly two cases, and both are
recognised here:

  1. this pull request ARCHIVES the change the issue tracks -- the issue's life
     really does end with it
  2. the issue is an ordinary filed one that no change tracks

FAILS OPEN ON WHAT IT CANNOT READ, exactly as the other gates do: no body, no
sidecars, an unreadable diff. A guard that blocks work it does not understand
gets switched off, and then it enforces nothing.
"""
from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys

# GitHub's own closing keywords, as documented. `Refs #12` is deliberately absent
# -- that is the form this guard exists to steer people towards.
CLOSING = re.compile(
    r"\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+#(\d+)", re.I
)
ARCHIVED_DIR = re.compile(r"^\d{4}-\d{2}-\d{2}-(.+)$")


def tracked_issues(root: pathlib.Path) -> dict[int, str]:
    """issue number -> change name, from every sidecar in the tree.

    Archived changes are included: their sidecar travels with them, and a pull
    request may legitimately close their issue.
    """
    found: dict[int, str] = {}
    for sidecar in root.glob("openspec/changes/**/.github-issue"):
        try:
            number = int("".join(c for c in sidecar.read_text() if c.isdigit()))
        except (OSError, ValueError):
            continue
        name = sidecar.parent.name
        if m := ARCHIVED_DIR.match(name):
            name = m.group(1)
        found[number] = name
    return found


def archived_by(diff_range: str, root: pathlib.Path) -> set[str]:
    """Changes this diff moves into openspec/changes/archive/."""
    try:
        out = subprocess.run(
            ["git", "diff", "--name-only", diff_range],
            cwd=root, capture_output=True, text=True, check=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError):
        return set()

    names = set()
    for path in out.splitlines():
        parts = path.split("/")
        if len(parts) > 4 and parts[:3] == ["openspec", "changes", "archive"]:
            if m := ARCHIVED_DIR.match(parts[3]):
                names.add(m.group(1))
    return names


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--body-file", type=pathlib.Path, required=True)
    ap.add_argument("--range", dest="diff_range", required=True)
    ap.add_argument("--root", type=pathlib.Path, default=pathlib.Path("."))
    args = ap.parse_args()

    try:
        body = args.body_file.read_text(encoding="utf-8")
    except OSError:
        print("pr-closes-guard: no pull request body to read; nothing to check")
        return 0

    closes = {int(n) for n in CLOSING.findall(body)}
    tracked = tracked_issues(args.root)
    archived = archived_by(args.diff_range, args.root)

    # THE OTHER DIRECTION, AND THE ONE THAT WENT WRONG TWICE. An archive pull
    # request that names no `Closes #<n>` leaves the tracking issue open forever
    # once the script's separate `close` step is forgotten — #38 and #67 sat
    # open under `opsx:archived` until somebody asked. GitHub closes on the
    # keyword and on nothing else, so the keyword is required exactly where the
    # rule says it is right.
    owed = sorted((n, c) for n, c in tracked.items() if c in archived and n not in closes)
    if owed:
        listed = "\n".join(f"    Closes #{n}   ->   {c}" for n, c in owed)
        print(f"""
This pull request ARCHIVES a change and does not close its tracking issue:

{listed}

A change's tracking issue closes at archive, and GitHub closes it only on the
keyword. Add the line above to the pull request body.

See .claude/rules/worktree-delivery.md.""", file=sys.stderr)
        return 1

    if not closes:
        print("pr-closes-guard: this pull request closes no issue by keyword")
        return 0

    bad = []
    for number in sorted(closes):
        change = tracked.get(number)
        if change is None:
            print(f"  ok       #{number} is not a change's tracking issue")
        elif change in archived:
            print(f"  ok       #{number} tracks {change}, which this pull request archives")
        else:
            bad.append((number, change))
            print(f"  REFUSED  #{number} tracks {change}, which this pull request does not archive")

    if not bad:
        return 0

    listed = "\n".join(f"    Closes #{n}   ->   {c}" for n, c in bad)
    print(f"""
A change's tracking issue closes at ARCHIVE, not at merge:

{listed}

Merging this would close an issue that is meant to follow its change through
applying, review and archiving — and GitHub would do it silently, in a thread
nobody is reading.

Write `Refs #<n>` instead. `Closes #<n>` is right only when this pull request
archives the change, or when the issue is an ordinary filed one no change tracks.

See .claude/rules/worktree-delivery.md.""", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
