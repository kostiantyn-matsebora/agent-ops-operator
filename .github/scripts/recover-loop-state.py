#!/usr/bin/env python3
"""Correct `loop:running` when the round it belonged to was never reported.

`gate` stamps `loop:running` the moment a round starts (`review-dispatch.yml`),
and `land` is the ONLY job that ever moves it to a terminal value
(`stalled` / `capped` / `mergeable`). That pairing assumes the run `gate`
started always reaches `land` -- true for a round that runs, fails, or times
out (`land`'s own `if:` already covers `success` / `failure` / `cancelled`,
see the `fix` job's `timeout-minutes` comment), but NOT for a run that never
starts running its jobs at all.

MEASURED LIVE ON #243, 2026-09-23: two `review-dispatch` runs completed with
conclusion `cancelled` and ZERO JOBS -- superseded by a newer trigger in the
same `concurrency` group before `gate` (the first job) ever started. `gate`'s
own `running` stamp from an EARLIER, actually-started run was left standing,
and nothing corrected it: `refresh-loop-state.py` deliberately does nothing
while it sees `loop:running` (a round may genuinely be in flight), so it is
the wrong tool for a stamp that belongs to a run which was itself discarded.

THIS PROGRAM IS THE OTHER HALF, and it only ever acts on `loop:running`. It
re-derives the true state the same way `land-dispatch.py`'s `Round` class
does -- round and grant COMMENTS since the label was placed, against
`max_rounds` from the vocabulary file -- and, separately, whether a review
thread is currently open. Three outcomes, decided in this order:

  1. Rounds used > cap                    -> `capped`
  2. A review thread is open               -> `stalled`
  3. Neither                               -> left alone

Case 3 is deliberate, not a gap. `loop:running` with rounds still available
and no thread open is genuinely ambiguous from state alone -- it could be a
round in flight RIGHT NOW, which this program must never downgrade. The
zero-jobs signature is what makes this program worth calling at all: pass
`--only-if-superseded`, and it exits 0 without touching anything unless the
completed run's own conclusion was `cancelled` with no jobs, which is the
one shape state alone cannot distinguish from "genuinely running".

EXITS 0 ALWAYS. Every failure to read `gh` leaves the label as it is rather
than guessing.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

STATE_SCRIPT = pathlib.Path(".github/scripts/conveyor-state.py")
CHECK_SCRIPT = pathlib.Path(".github/scripts/review-not-clean.py")
DEFAULT_VOCABULARY = pathlib.Path(".github/review-triage.json")


def gh(*args: str) -> str:
    return subprocess.run(["gh", *args], capture_output=True, text=True, check=True).stdout


def load_markers(path: pathlib.Path) -> dict:
    try:
        doc = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError):
        doc = {}
    return {
        "round": doc.get("round_marker", "<!-- conveyor:round"),
        "grant": doc.get("grant_marker", "<!-- conveyor:grant -->"),
        "max_rounds": doc.get("max_rounds", 5),
    }


def comments_since(repo: str, pr: int, since: str) -> list[dict]:
    try:
        raw = json.loads(gh("api", f"repos/{repo}/issues/{pr}/comments", "--paginate") or "[]")
    except (subprocess.CalledProcessError, json.JSONDecodeError):
        return []
    return [c for c in raw if not since or (c.get("created_at") or "") >= since]


def count_marked(comments: list[dict], marker: str) -> int:
    return sum(1 for c in comments if marker in (c.get("body") or ""))


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--since", required=True, help="when the current loop:running label was placed (ISO 8601)")
    ap.add_argument("--only-if-superseded", action="store_true",
                     help="skip everything unless the completing run's own conclusion was `cancelled` with "
                          "zero jobs -- the one shape state alone cannot tell apart from a round genuinely "
                          "in flight. Callers pass the completing run's own facts here rather than this "
                          "program re-fetching a run it was not told about.")
    ap.add_argument("--run-conclusion", default="", help="the completing workflow_run's own conclusion")
    ap.add_argument("--run-job-count", type=int, default=-1, help="how many jobs that run reported")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    args = ap.parse_args()

    if args.only_if_superseded and not (args.run_conclusion == "cancelled" and args.run_job_count == 0):
        print(f"#{args.pr}: the completing run was not a superseded (zero-job, cancelled) one; nothing to recover")
        return 0

    if not CHECK_SCRIPT.is_file() or not STATE_SCRIPT.is_file():
        print(f"::notice::#{args.pr}: could not recover the loop label, a helper script is missing")
        return 0

    try:
        current = gh("pr", "view", str(args.pr), "--repo", args.repo, "--json", "labels", "--jq", ".labels[].name")
    except subprocess.CalledProcessError as exc:
        print(f"::notice::#{args.pr}: could not read the current labels ({exc}); leaving the loop label as it is")
        return 0
    if "loop:running" not in current.splitlines():
        print(f"#{args.pr}: loop label is not `running`; nothing for this program to recover")
        return 0

    markers = load_markers(args.vocabulary)
    seen = comments_since(args.repo, args.pr, args.since)
    rounds_used = count_marked(seen, markers["round"])
    grants = count_marked(seen, markers["grant"])
    cap = markers["max_rounds"] * (1 + grants)

    if rounds_used > cap:
        state, why = "capped", f"{rounds_used} rounds used, cap is {cap}"
    else:
        checked = subprocess.run([sys.executable, str(CHECK_SCRIPT), "--repo", args.repo, "--pr", str(args.pr)],
                                  capture_output=True, text=True)
        if checked.returncode == 1:
            state, why = "stalled", "a review thread is open"
        elif checked.returncode == 0:
            print(f"#{args.pr}: {rounds_used} of {cap} rounds used, no thread open; "
                  "a round may genuinely be in flight, leaving `running` as it is")
            return 0
        else:
            print(f"::notice::#{args.pr}: could not read the review threads (review-not-clean.py exited "
                  f"{checked.returncode}); leaving the loop label as it is")
            return 0

    result = subprocess.run([sys.executable, str(STATE_SCRIPT), "--repo", args.repo, "--target", str(args.pr),
                             "--loop", state, "--vocabulary", str(args.vocabulary)], check=False)
    if result.returncode == 0:
        print(f"#{args.pr}: the round that held `running` was superseded before it could report ({why}); "
              f"corrected the loop label to {state}")
    else:
        print(f"::notice::#{args.pr}: {why}, but conveyor-state.py could not run (exit {result.returncode}); "
              "the label was NOT corrected")
    return 0


if __name__ == "__main__":
    sys.exit(main())
