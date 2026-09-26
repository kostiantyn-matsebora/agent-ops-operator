#!/usr/bin/env python3
"""Correct a pull request's `loop:mergeable` label if it has gone stale.

STATE, NEVER A CHECK'S VERDICT -- the same distinction `review-not-clean.py`'s
own docstring draws. `carry.py` sets `loop:mergeable` at the moment a green
`ci` finds no review thread open. That moment passes, and a LATER review
completion can post new findings without ever touching the label, because
nothing without the `conveyor:fix` grant re-checks it. Measured live on #226
(manually driven, since it edits `review-dispatch.yml` itself and the loop
cannot run on it): the label said `mergeable` for over an hour after three
findings landed.

Called from `dispatch-gate.py` on every `claude-review` completion that does
NOT start a round. THIS PROGRAM DECIDES NOTHING ABOUT THE LABEL. It reads whether
a review thread is open and sends the loop machine the event `thread:opened`;
the machine's table says that moves `mergeable` to `stalled` and moves nothing
else, and in particular leaves `running` alone, since a round from an earlier
trigger may still be in flight and owns the label through its own ending.

EXITS 0 ALWAYS. A transient failure to read the threads leaves the label as it
is rather than guessing.
"""
from __future__ import annotations

import argparse
import pathlib
import subprocess
import sys

# CWD-RELATIVE, LIKE `carry.py`'s OWN CALLS TO THESE TWO SCRIPTS --
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

    checked = subprocess.run([sys.executable, str(CHECK_SCRIPT), "--repo", args.repo, "--pr", str(args.pr)],
                              capture_output=True, text=True)
    # THREE OUTCOMES, NOT TWO. Exit 0 is "no thread open" and exit 1 is "a
    # thread is open", `review-not-clean.py`'s own verdicts. ANYTHING ELSE is
    # that script crashing before it reached one (an unhandled `gh` failure
    # raises a traceback and also exits 1), and a transient API failure must
    # not stall a label that was never shown to be stale.
    if checked.returncode == 0:
        print(f"#{args.pr}: no review thread open; leaving the loop label as it is")
        return 0
    if checked.returncode != 1:
        print(f"::notice::#{args.pr}: could not read the review threads (review-not-clean.py exited "
              f"{checked.returncode}); leaving the loop label as it is")
        return 0

    result = subprocess.run([sys.executable, str(STATE_SCRIPT), "--repo", args.repo, "--target", str(args.pr),
                             "--loop-event", "thread:opened"], check=False)
    # `conveyor-state.py` ITSELF ALWAYS EXITS 0, but that only covers what it
    # does once it runs. NONZERO HERE means the interpreter or the script could
    # not run at all, and "corrected" would be a claim this program cannot back.
    if result.returncode == 0:
        print(f"#{args.pr}: a review thread is open; sent thread:opened to the loop machine")
    else:
        print(f"::notice::#{args.pr}: a review thread is open, but conveyor-state.py could not run "
              f"(exit {result.returncode}); the label was NOT corrected")
    return 0


if __name__ == "__main__":
    sys.exit(main())
