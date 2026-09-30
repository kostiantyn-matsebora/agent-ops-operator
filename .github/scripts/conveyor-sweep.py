#!/usr/bin/env python3
"""Resume a WAITING fixing loop whose disputes have all been answered.

A loop ends `waiting` when every item left is a dispute the fixing step made, or
a review thread is open, and a person is owed an answer. Two answers exist:

  a comment    an Actions event -- `review-dispatch.yml` starts a round on it
  a resolution the thread resolved, the finding dismissed -- NOT an Actions
               event (`pull_request_review_thread` is a webhook only; the
               workflow naming it is refused, see .claude/rules/gotchas.md)

So nothing hears the second answer. Measured on #259: the person resolved the
disputed thread, and the next round -- an hour later, started by hand -- still
reported a person's answer as owed. THIS PROGRAM IS WHAT HEARS IT, on a schedule:
for every open pull request carrying the fix label and the `waiting` label, it
re-reads the disputes the same way the archive guard does
(`conveyor_io.unanswered_disputes`, over the unresolved threads and the pull
request's comments) and, when none is unanswered, dispatches one round. The
gate then re-reads the grant and the head's runs as for every start; this
program decides nothing about whether the round may run, only that the wait is
over.

EVERYTHING ELSE IS LEFT ALONE. A pull request still waiting stays waiting. One
whose loop is running, capped, stalled or mergeable is not this program's. Every
`gh` failure is a notice and exit 0: a sweep that fails a run nobody watches
helps nobody, and the next sweep re-reads everything from the live state.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor_io  # noqa: E402  -- the dispute-thread walk: the archive guard reads the same one

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
WORKFLOW = "review-dispatch.yml"


class Unreadable(Exception):
    """Something the sweep cannot read. The pull request is left as it is."""


def gh(*args: str) -> str:
    out = subprocess.run(["gh", *args], capture_output=True, text=True)
    if out.returncode != 0:
        raise Unreadable(f"gh {args[0]} {args[1] if len(args) > 1 else ''}: {(out.stderr or '').strip()}")
    return out.stdout


def gh_json(*args: str):
    try:
        return json.loads(gh(*args) or "null")
    except json.JSONDecodeError as exc:
        raise Unreadable(f"unreadable JSON from gh {args[0]}: {exc}")


def waiting_pull_requests(repo: str, fix: str, waiting: str) -> list[int]:
    rows = gh_json("pr", "list", "--repo", repo, "--state", "open", "--limit", "200",
                   "--label", fix, "--label", waiting, "--json", "number") or []
    return sorted(int(r["number"]) for r in rows if r.get("number") is not None)


def unanswered(repo: str, pr: int, marker: str) -> list[str]:
    """What still waits on a person: the unresolved disputed threads with no
    person's comment after the marker, and the pull request comments where a
    dispute of an analysis issue or a check stands unanswered.

    THE WALK IS `conveyor_io`'S -- shared with the archive guard, which reads
    the exact same two-part shape. A `RuntimeError` from that shared read is
    an `Unreadable` here, this program's own vocabulary for "left as it is"."""
    try:
        return conveyor_io.unanswered_disputes(repo, pr, marker,
                                      comment_note="a pull request comment disputing analysis issues or checks")
    except RuntimeError as exc:
        raise Unreadable(str(exc))


def sweep(repo: str, vocab: dict, dry_run: bool = False) -> int:
    fix, waiting = vocab["approve_label"], vocab["loop_labels"]["waiting"]
    marker = vocab["dispute_marker"]
    try:
        prs = waiting_pull_requests(repo, fix, waiting)
    except Unreadable as exc:
        print(f"::notice::the waiting pull requests could not be listed ({exc}); nothing swept")
        return 0
    if not prs:
        print(f"no open pull request carries both `{fix}` and `{waiting}`; nothing to resume")
        return 0
    for pr in prs:
        try:
            left = unanswered(repo, pr, marker)
        except Unreadable as exc:
            print(f"::notice::#{pr}: could not read its disputes ({exc}); left waiting")
            continue
        if left:
            print(f"#{pr}: still waiting on {len(left)} unanswered dispute(s): {', '.join(left)}")
            continue
        if dry_run:
            print(f"#{pr}: every dispute is answered or resolved; a round would start (dry run)")
            continue
        try:
            gh("workflow", "run", WORKFLOW, "--repo", repo, "-f", f"pr={pr}", "-f", "mode=all")
        except Unreadable as exc:
            print(f"::notice::#{pr}: every dispute is answered, but the round could not be started ({exc}); "
                  "the next sweep tries again")
            continue
        print(f"#{pr}: every dispute is answered or resolved; started a round")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    ap.add_argument("--dry-run", action="store_true", help="say what would start, start nothing")
    args = ap.parse_args()
    try:
        vocab = json.loads(args.vocabulary.read_text())
    except (OSError, ValueError) as exc:
        print(f"::notice::no vocabulary to read ({exc}); nothing swept")
        return 0
    return sweep(args.repo, vocab, args.dry_run)


if __name__ == "__main__":
    sys.exit(main())
