#!/usr/bin/env python3
"""Correct a pull request's `loop:mergeable` label if it has gone stale.

STATE, NEVER A CHECK'S VERDICT -- the same distinction `review-not-clean.py`'s
own docstring draws. `carry-from-pr.sh` sets `loop:mergeable` at the moment a
green `ci` finds no review thread open; that moment passes, and a LATER
review completion can post new findings without ever touching the label,
because nothing without the `conveyor:fix` grant re-checks it. Measured live
on #226 (manually driven -- it edits `review-dispatch.yml` itself, so the
loop cannot run on it): the label said `mergeable` for over an hour after
three findings landed.

Called from `review-dispatch.yml`'s `gate` job on every `claude-review`
completion that does NOT start a round (`mode=none`) -- a round that DOES
start already owns the label through its own ending. Reading the threads
here to correct a label is not the frozen-check problem `review-not-clean.py`
exists to keep out of `ci-green`: nothing required reads this value, and it
is re-asserted at the next transition regardless.

EXITS 0 ALWAYS. A transient failure to read the threads leaves the label as
it is rather than guessing.
"""
from __future__ import annotations

import argparse
import pathlib
import subprocess
import sys

# CWD-RELATIVE, LIKE `carry-from-pr.sh`'s OWN CALLS TO THESE TWO SCRIPTS --
# not `__file__`-relative. CI checks this repository out at the working
# directory and runs every script from there, and a `__file__`-relative path
# would resolve to THIS repository's checkout even when a test substitutes a
# stub at a different one.
STATE_SCRIPT = pathlib.Path(".github/scripts/conveyor-state.py")
CHECK_SCRIPT = pathlib.Path(".github/scripts/review-not-clean.py")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    args = ap.parse_args()

    if not CHECK_SCRIPT.is_file() or not STATE_SCRIPT.is_file():
        print(f"::notice::#{args.pr}: could not refresh the loop label, a helper script is missing")
        return 0

    clean = subprocess.run([sys.executable, str(CHECK_SCRIPT), "--repo", args.repo, "--pr", str(args.pr)],
                            capture_output=True, text=True).returncode == 0
    if clean:
        # LEAVE IT. A clean pull request's label may already be `mergeable`
        # from an earlier ci success, or may be something else entirely (a
        # round never ran here at all) -- either is a fact this program has
        # no new information about.
        print(f"#{args.pr}: no review thread open; leaving the loop label as it is")
        return 0

    subprocess.run([sys.executable, str(STATE_SCRIPT), "--repo", args.repo, "--target", str(args.pr),
                    "--loop", "stalled"], check=False)
    print(f"#{args.pr}: a review thread is open; corrected the loop label to stalled")
    return 0


if __name__ == "__main__":
    sys.exit(main())
