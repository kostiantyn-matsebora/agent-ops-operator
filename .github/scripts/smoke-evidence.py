#!/usr/bin/env python3
"""Has the tagged commit already been smoked? Look before provisioning one.

A tag push runs `release.yml` and nothing else, and a release is many tags on
one commit -- fourteen on 2026-09-06, each provisioning its own cluster for a
smoke that proves the COMMIT and nothing about the tag that triggered it. Both
failures of that release were LOAD: a k3d image import deadlocked under
fourteen concurrent runs, and the console lifecycle lane timed out on a step
that takes eight seconds when the runners are quiet.

`ci_is_green` already answers this shape of question for `ci.yml` -- look up
the commit's existing run rather than trusting the tag. This does the same for
the smoke, over CHECK RUNS rather than workflow runs: the smoke is a reusable
workflow called by a job named `smoke`, so every smoke -- from `release.yml` or
from `e2e-smoke.yml` -- appears on the commit as a check run named
`smoke / e2e / smoke`. The commit's check runs are the one place all of them
meet, whatever workflow produced them. Matching the TAIL (`e2e / smoke`) rather
than the full name keeps a renamed caller job from silently switching reuse
off.

Classification, per the design:

  a check run with conclusion "success"           -> smoked
  none, or only failed ones                        -> not smoked, run one
  one "queued" or "in_progress" and no success      -> wait, then re-classify
  the wait bound passes with one still running      -> not smoked, run our own
  `gh` unreachable or the API erring                -> not smoked -- the safe
                                                        answer is to run the
                                                        smoke, never to publish
                                                        on missing evidence

Prints `smoked=true` or `smoked=false` to stdout (nothing else on that stream,
so a workflow can capture it directly into `$GITHUB_OUTPUT`) and a one-line
reason to stderr.
"""
from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time

SUFFIX = "e2e / smoke"


def check_runs(repo: str, sha: str) -> list[dict]:
    """Every check run on the commit, paginated. Raises on a `gh` failure --
    the caller treats that as "not smoked", never as "no check runs"."""
    runs: list[dict] = []
    page = 1
    while True:
        # `--method GET` is NOT decoration: without it, `gh api` sends `-f`
        # params as a request body rather than a query string on some routes,
        # and this one answers a bodied GET with 404 rather than the list —
        # confirmed against the live API, not merely read off the docs.
        out = subprocess.run(
            ["gh", "api", "--method", "GET", f"repos/{repo}/commits/{sha}/check-runs",
             "-f", f"per_page=100", "-f", f"page={page}"],
            capture_output=True, text=True,
        )
        if out.returncode != 0:
            raise RuntimeError(out.stderr.strip() or "gh api failed")
        payload = json.loads(out.stdout or "{}")
        page_runs = payload.get("check_runs", []) or []
        runs.extend(page_runs)
        if len(page_runs) < 100:
            return runs
        page += 1


def classify(runs: list[dict]) -> str:
    """`smoked` | `pending` | `not smoked`, over the runs whose name ends
    with the smoke's check-run suffix."""
    smoke_runs = [r for r in runs if (r.get("name") or "").endswith(SUFFIX)]
    if any(r.get("conclusion") == "success" for r in smoke_runs):
        return "smoked"
    if any(r.get("status") != "completed" for r in smoke_runs):
        return "pending"
    return "not smoked"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--sha", required=True, help="the tagged commit")
    ap.add_argument("--wait-minutes", type=int, default=20,
                     help="how long to wait for an in-flight smoke on the commit before giving up and running one")
    ap.add_argument("--poll-seconds", type=int, default=30,
                     help="interval between re-checks while waiting; lowered by the suite, never by the workflow")
    args = ap.parse_args()

    deadline = time.monotonic() + args.wait_minutes * 60
    waited = False
    while True:
        try:
            state = classify(check_runs(args.repo, args.sha))
        except (RuntimeError, OSError, json.JSONDecodeError) as exc:
            print(f"smoked=false", file=sys.stdout)
            print(f"the check-run lookup failed ({exc}); running a smoke rather than publishing on missing evidence",
                  file=sys.stderr)
            return 0

        if state == "smoked":
            print("smoked=true")
            print(f"a smoke already passed for {args.sha}"
                  + (" (found after waiting on an in-flight one)" if waited else ""), file=sys.stderr)
            return 0

        if state == "not smoked":
            print("smoked=false")
            print(f"no passed smoke for {args.sha}"
                  + (" (the one in flight did not succeed)" if waited else " and none in flight"),
                  file=sys.stderr)
            return 0

        # pending: an in-flight smoke on this commit. Wait for its verdict
        # rather than racing it -- tags are pushed three at a time by rule, so
        # this is the common case, not the exception.
        if time.monotonic() >= deadline:
            print("smoked=false")
            print(f"a smoke is still running for {args.sha} after {args.wait_minutes}m; "
                  "running our own rather than waiting forever", file=sys.stderr)
            return 0
        waited = True
        print(f"a smoke is in flight for {args.sha}; waiting ({args.poll_seconds}s)...", file=sys.stderr)
        time.sleep(args.poll_seconds)


if __name__ == "__main__":
    sys.exit(main())
