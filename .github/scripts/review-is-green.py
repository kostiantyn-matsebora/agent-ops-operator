#!/usr/bin/env python3
"""Has `claude-review` finished for this commit, and how did it conclude?

`ci-green` names every job it needs, and `claude-review.yml` is a separate
top-level workflow -- `needs:` cannot cross workflow files, so the only way
for the review's verdict to reach the one required check is a job in `ci.yml`
that asks the platform directly, the same shape `smoke-evidence.py` already
uses for a different question on the same commit.

UNLIKE THE SMOKE, THIS NEVER FALLS BACK TO RUNNING ITS OWN COPY -- there is
nothing this job could run instead of the review, so a review that never
started or a lookup that fails must both be visible failures rather than a
silent "treat as clean". The review runs on every `pull_request` event with
no path filter (`claude-review.yml`), so the ordinary case is always a run to
find, and its absence past the wait bound is worth failing loudly on rather
than guessing.

Classification:

  a run for this sha with conclusion "success"     -> green
  a run for this sha with any other conclusion      -> not green, printed
  a run still queued or in progress, and no other    -> wait, then re-classify
  the wait bound passes with one still running       -> not green (timeout)
  none at all, past the wait bound                    -> not green (never ran)
  `gh` unreachable or the API erring                  -> not green (fail closed)

Prints `green=true` or `green=false` to stdout, and a one-line reason plus
(for a non-success conclusion) the run's URL to stderr.
"""
from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time

WORKFLOW = "claude-review.yml"


def runs_for_sha(repo: str, sha: str, api_timeout: int) -> list[dict]:
    out = subprocess.run(
        ["gh", "api", "--method", "GET", f"repos/{repo}/actions/workflows/{WORKFLOW}/runs",
         "-f", f"head_sha={sha}", "-f", "per_page=10"],
        capture_output=True, text=True, timeout=api_timeout,
    )
    if out.returncode != 0:
        raise RuntimeError(out.stderr.strip() or "gh api failed")
    return json.loads(out.stdout or "{}").get("workflow_runs", []) or []


def classify(runs: list[dict]) -> tuple[str, dict | None]:
    """(`green` | `pending` | `not green` | `none`, the deciding run or None).

    The LATEST run for the sha decides -- a superseded run cancelled by this
    workflow's own concurrency group must never out-vote the one that
    actually finished."""
    if not runs:
        return "none", None
    latest = max(runs, key=lambda r: r.get("run_started_at") or r.get("created_at") or "")
    if latest.get("status") != "completed":
        return "pending", latest
    if latest.get("conclusion") == "success":
        return "green", latest
    return "not green", latest


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--sha", required=True, help="the commit ci-green is evaluating")
    ap.add_argument("--wait-minutes", type=int, default=20,
                     help="how long to wait for the review to finish before giving up")
    ap.add_argument("--poll-seconds", type=int, default=30,
                     help="interval between re-checks while waiting; lowered by the suite, never by the workflow")
    ap.add_argument("--api-timeout", type=int, default=30,
                     help="seconds before a single gh api call is treated as hung; lowered by the suite, never by the workflow")
    args = ap.parse_args()

    deadline = time.monotonic() + args.wait_minutes * 60
    waited = False
    while True:
        try:
            state, run = classify(runs_for_sha(args.repo, args.sha, args.api_timeout))
        except (RuntimeError, OSError, subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
            print("green=false")
            print(f"the workflow-run lookup failed ({exc}); not green -- fail closed rather than "
                  "publish on missing evidence", file=sys.stderr)
            return 0

        if state == "green":
            print("green=true")
            print(f"claude-review succeeded for {args.sha}"
                  + (" (found after waiting)" if waited else ""), file=sys.stderr)
            return 0

        if state == "not green":
            print("green=false")
            print(f"claude-review concluded {run.get('conclusion')} for {args.sha}: {run.get('html_url')}",
                  file=sys.stderr)
            return 0

        if state == "none" and time.monotonic() >= deadline:
            print("green=false")
            print(f"no claude-review run ever appeared for {args.sha} within {args.wait_minutes}m", file=sys.stderr)
            return 0

        if state == "pending" and time.monotonic() >= deadline:
            print("green=false")
            print(f"claude-review is still running for {args.sha} past the {args.wait_minutes}m wait bound: "
                  f"{run.get('html_url')}", file=sys.stderr)
            return 0

        waited = True
        time.sleep(args.poll_seconds)


if __name__ == "__main__":
    sys.exit(main())
