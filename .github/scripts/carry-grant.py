#!/usr/bin/env python3
"""Carry an issue's standing instruction forward to the next station's label.

A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER MINT ONE.
This is the whole fix for #201: a remote session opened its pull request
carrying `autofix` because its own instructions said to, the fixing loop's
gate asked whether the labeller may push here, the labeller was `claude[bot]`,
the answer was no, and the label was stripped with a refusal comment. The gate
was right; a session must never place a label that authorises anything.

WHAT THIS PROGRAM DOES INSTEAD: reads the tracking ISSUE's current labels
(never the pull request's -- the issue is where the standing instruction
lives), and if `run_label` (`conveyor:run`) is there, checks that whoever
placed it can still push here, and places the STATION's label on whichever
object that station is driven from:

  fix       the pull request -- `--pr` is REQUIRED. The fixing loop's gate
            reads the pull request's own labels.
  archive   the ISSUE -- `--pr` is REFUSED. The pull request that carried the
            change is MERGED and CLOSED by the time this station runs, and a
            label on a closed pull request drives nothing; the tracking issue
            is what survives every station.

THE LANE IS READ BEFORE ARCHIVE PLACES ANYTHING. An issue not bound to an
openspec change (no `openspec/changes/*/.github-issue` naming it, no `opsx:`
phase label) is on the PLAIN lane, which has no archive station -- its line
ends at the merge this program was called to react to. Placing a label
nothing consumes would be a station that does not exist.

EXITS 0 DOING NOTHING WHEN THE STANDING INSTRUCTION IS ABSENT -- that is the
ordinary case (a single-station label drove this pull request or issue by
hand, or nobody granted the standing instruction at all), never an error.

RE-CHECKED, NEVER TRUSTED. `run_label`'s placer is read from the issue's
timeline at the moment this program runs, exactly as `review-dispatch.yml`'s
own gate re-checks a label it did not itself place -- a carried label is only
as good as the grant it claims to carry, and that grant is re-verified here
rather than assumed because a workflow is the one calling this program.

ONE COMMENT, ONCE, under a marker naming whose instruction was carried --
so a reader sees whose decision it was, on the object the label reached.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

MARKER = "<!-- carry-grant:{station} -->"
MAY_PUSH = {"admin", "maintain", "write"}
DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"


def gh(*args: str, check: bool = True) -> str:
    out = subprocess.run(["gh", *args], capture_output=True, text=True)
    if check and out.returncode != 0:
        raise RuntimeError(f"gh {' '.join(args)}: {out.stderr.strip()}")
    return out.stdout.strip()


def vocabulary(path: pathlib.Path) -> dict:
    return json.loads(path.read_text())


def issue_labels(repo: str, issue: int) -> set[str]:
    raw = gh("issue", "view", str(issue), "--repo", repo, "--json", "labels")
    try:
        doc = json.loads(raw or "{}")
    except json.JSONDecodeError:
        return set()
    return {l.get("name") for l in doc.get("labels") or [] if l.get("name")}


def label_placement(repo: str, issue: int, label: str) -> tuple[str, str] | None:
    """(login, iso timestamp) of the LATEST placement of `label` on the
    issue's timeline, or None if the timeline shows nobody placing it. The
    latest counts, exactly as review-dispatch.yml's own gate reads it."""
    raw = gh("api", f"repos/{repo}/issues/{issue}/timeline", "--paginate")
    try:
        events = json.loads(raw or "[]")
    except json.JSONDecodeError:
        return None
    placements = [e for e in events
                  if e.get("event") == "labeled" and (e.get("label") or {}).get("name") == label]
    if not placements:
        return None
    last = placements[-1]
    login = (last.get("actor") or {}).get("login")
    when = last.get("created_at")
    if not login or not when:
        return None
    return login, when


def permission(repo: str, login: str) -> str:
    try:
        return gh("api", f"repos/{repo}/collaborators/{login}/permission", "--jq", ".permission")
    except RuntimeError:
        return "none"


def is_opsx_lane(repo: str, issue: int) -> bool:
    """An issue is on the opsx lane when a change's .github-issue names it, or
    when it carries an opsx: phase label -- both FACTS a program reads, never
    a judgement about how the issue is worded."""
    for sidecar in pathlib.Path(".").glob("openspec/changes/*/.github-issue"):
        try:
            number = int("".join(c for c in sidecar.read_text() if c.isdigit()))
        except (OSError, ValueError):
            continue
        if number == issue:
            return True
    labels = issue_labels(repo, issue)
    return any(l.startswith("opsx:") for l in labels)


def already_carried(repo: str, target: int, marker: str) -> bool:
    # A PULL REQUEST IS AN ISSUE TO THIS API, so one call reads either target's
    # comments.
    raw = gh("api", f"repos/{repo}/issues/{target}/comments", "--paginate", "--jq", ".[].body")
    return marker in raw


def comment(repo: str, target: int, body: str) -> None:
    subprocess.run(["gh", "issue", "comment", str(target), "--repo", repo, "--body", body], check=False)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--issue", type=int, required=True, help="the tracking issue carrying the standing instruction")
    ap.add_argument("--station", choices=["fix", "archive"], required=True)
    ap.add_argument("--pr", type=int, help="required for --station fix; refused for --station archive")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    args = ap.parse_args()

    if args.station == "fix" and args.pr is None:
        print("::error::--station fix requires --pr: the label goes on that pull request", file=sys.stderr)
        return 2
    if args.station == "archive" and args.pr is not None:
        print("::error::--station archive refuses --pr: the label goes on the issue, and a pull request "
              "number there names a closed object nothing reads", file=sys.stderr)
        return 2

    vocab = vocabulary(args.vocabulary)
    run_label = vocab["run_label"]
    station_label = {"fix": vocab["approve_label"], "archive": vocab["archive_label"]}[args.station]

    labels = issue_labels(args.repo, args.issue)
    if run_label not in labels:
        print(f"#{args.issue} does not carry `{run_label}`; nothing to carry, and that is the ordinary case")
        return 0

    if args.station == "archive" and not is_opsx_lane(args.repo, args.issue):
        print(f"#{args.issue} is on the plain lane, which has no archive station; its line ended at the merge")
        return 0

    placed = label_placement(args.repo, args.issue, run_label)
    if placed is None:
        print(f"::notice::#{args.issue} carries `{run_label}` but the timeline shows nobody placing it; "
              "nothing carried")
        return 0
    placer, since = placed
    perm = permission(args.repo, placer)
    if perm not in MAY_PUSH:
        print(f"::notice::#{args.issue}'s `{run_label}` was placed by {placer}, who now has `{perm}`; "
              "nothing carried")
        return 0

    marker = MARKER.format(station=args.station)
    target_kind = "pr" if args.station == "fix" else "issue"
    target = args.pr if args.station == "fix" else args.issue
    if already_carried(args.repo, target, marker):
        print(f"#{target} already carries the `{args.station}` grant; not commenting again")
        return 0

    gh("issue", "edit", str(target), "--repo", args.repo, "--add-label", station_label)
    where = "pull request" if args.station == "fix" else "issue"
    comment(args.repo, target,
            f"{marker}\n"
            f"Placed `{station_label}` on this {where}, carrying @{placer}'s standing instruction "
            f"(`{run_label}`, placed on #{args.issue} at {since}). This program relays a grant a "
            f"person already gave; it does not decide anything on its own.")
    print(f"carried `{run_label}` (from {placer}) to `{station_label}` on {target_kind} #{target}")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except RuntimeError as exc:
        print(f"::error::{exc}", file=sys.stderr)
        sys.exit(1)
