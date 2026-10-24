#!/usr/bin/env python3
"""Apply one EVENT to an issue's station or a pull request's loop state.

THE ONLY WRITER OF A STATE LABEL, AND IT TAKES EVENTS, NEVER VALUES. A caller
says what happened (`round:start`, `merge:finished_carried`). This reads the
target's live label, asks `conveyor.py`'s transition table what follows, and
writes that. An event the table skips leaves the label alone, which is how a
running round keeps its label against a green CI on an older head, and how
`done` stays done. A caller that could name a value could put the line in a
state the machine has no path to, which is what several programs did when each
held its own idea of the next state.

STATE, NOT A GRANT. `review-triage.json` names two families beside the
`conveyor:` grants: `station_labels` on the ISSUE and `loop_labels` on the PULL
REQUEST. Nothing reads them to decide anything. They exist so a person can read
where a change is without opening a run log. That is why this program NEVER
FAILS THE JOB THAT CALLS IT: a transition that could not be recorded is a
notice, never a broken station. The next transition re-derives from the live
label, which is what keeps a missed one from sticking.

AT MOST ONE VALUE PER FAMILY. Writing `fix` removes the other five in the same
edit, so two stations can never show at once. A target that somehow shows two
is read as its most advanced one and normalised by the next write.

A PULL REQUEST IS AN ISSUE TO THIS API, so one command serves both targets.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
FAMILIES = {"station": ("station_labels", conveyor.STATION_STATES, conveyor.station_next),
            "loop": ("loop_labels", conveyor.LOOP_STATES, conveyor.loop_next)}


def current(labels_on_target: set, family_labels: dict, order) -> str:
    """The state the target's labels say: the most advanced one present, or `none`."""
    present = [s for s in order if s != "none" and family_labels.get(s) in labels_on_target]
    return present[-1] if present else "none"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, allow_abbrev=False)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--target", type=int, required=True, help="the issue or pull request number")
    group = ap.add_mutually_exclusive_group(required=True)
    group.add_argument("--station-event", help="an event of the station machine, on the issue")
    group.add_argument("--loop-event", help="an event of the loop machine, on the pull request")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    args = ap.parse_args()

    family, event = ("station", args.station_event) if args.station_event else ("loop", args.loop_event)
    key, order, nxt = FAMILIES[family]
    try:
        labels = json.loads(args.vocabulary.read_text())[key]
    except (OSError, ValueError, KeyError) as exc:
        print(f"::notice::conveyor state not recorded: the vocabulary has no {key} ({exc})")
        return 0

    read = subprocess.run(["gh", "issue", "view", str(args.target), "--repo", args.repo,
                           "--json", "labels", "--jq", ".labels[].name"], capture_output=True, text=True)
    if read.returncode != 0:
        print(f"::notice::conveyor state not recorded on #{args.target}: the labels could not be read "
              f"({read.stderr.strip() or 'gh issue view failed'})")
        return 0
    state = current(set(read.stdout.split("\n")), labels, order)
    try:
        after = nxt(state, event)
    except ValueError as exc:
        print(f"::notice::conveyor state not recorded on #{args.target}: {exc}")
        return 0
    if after is None:
        print(f"#{args.target}: {family} = {state} ({event} does not move it)")
        return 0
    if after not in labels:
        print(f"::notice::conveyor state not recorded on #{args.target}: {after!r} has no label in {key}")
        return 0

    cmd = ["gh", "issue", "edit", str(args.target), "--repo", args.repo, "--add-label", labels[after]]
    for other, name in labels.items():
        if other != after:
            cmd += ["--remove-label", name]
    out = subprocess.run(cmd, capture_output=True, text=True)
    if out.returncode != 0:
        # A LABEL THAT DOES NOT EXIST IN THE REPOSITORY is the likely cause: the
        # families are created by hand, once. Say which, and move on.
        print(f"::notice::conveyor state not recorded on #{args.target} ({family} {state} -> {after}): "
              f"{out.stderr.strip() or 'gh issue edit failed'}")
        return 0
    print(f"#{args.target}: {family} = {labels[after]} (was {state}, {event})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
