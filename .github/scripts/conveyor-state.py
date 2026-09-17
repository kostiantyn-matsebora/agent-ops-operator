#!/usr/bin/env python3
"""Move an issue's or a pull request's conveyor STATE label to one value.

STATE, NOT A GRANT. `review-triage.json` names two families beside the
`conveyor:` grants: `station_labels` on the ISSUE (which station the line is
at) and `loop_labels` on the PULL REQUEST (what the fixing loop is doing).
Nothing reads them to decide anything -- they exist so a person can read where
a change is without opening a run log. That is why this program NEVER FAILS
THE JOB THAT CALLS IT: a transition that could not be recorded is a notice,
never a broken station. The next transition re-asserts its own value, which is
what keeps a missed one from sticking.

AT MOST ONE VALUE PER FAMILY. Setting `fix` removes `implement`, `merge`,
`archive` and `done` in the same edit, so two stations can never show at once.
Removing a label that is not there is a no-op to `gh issue edit`, so the call
is always safe to repeat.

A PULL REQUEST IS AN ISSUE TO THIS API, so one command serves both targets.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
FAMILIES = {"station": "station_labels", "loop": "loop_labels"}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--target", type=int, required=True, help="the issue or pull request number")
    group = ap.add_mutually_exclusive_group(required=True)
    group.add_argument("--station", help="the issue's station: implement, fix, merge, archive, done")
    group.add_argument("--loop", help="the pull request's loop state: running, stalled, capped, mergeable")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    args = ap.parse_args()

    family, value = ("station", args.station) if args.station else ("loop", args.loop)
    try:
        labels = json.loads(args.vocabulary.read_text())[FAMILIES[family]]
    except (OSError, ValueError, KeyError) as exc:
        print(f"::notice::conveyor state not recorded: the vocabulary has no {FAMILIES[family]} ({exc})")
        return 0
    if value not in labels:
        print(f"::notice::conveyor state not recorded: {value!r} is not a {family} value "
              f"({', '.join(labels)})")
        return 0

    cmd = ["gh", "issue", "edit", str(args.target), "--repo", args.repo, "--add-label", labels[value]]
    for other, name in labels.items():
        if other != value:
            cmd += ["--remove-label", name]
    out = subprocess.run(cmd, capture_output=True, text=True)
    if out.returncode != 0:
        # A LABEL THAT DOES NOT EXIST IN THE REPOSITORY is the likely cause: the
        # families are created by hand, once. Say which, and move on.
        print(f"::notice::conveyor state not recorded on #{args.target} ({family}={value}): "
              f"{out.stderr.strip() or 'gh issue edit failed'}")
        return 0
    print(f"#{args.target}: {family} = {labels[value]}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
