#!/usr/bin/env python3
"""Re-run the `review-clean` job of a pull request head's own `ci` run.

WHY A RE-RUN, AND NOT A RUN OF ITS OWN. `review-clean` reads the review's
threads LIVE, so a person resolving (or unresolving) a review-authored thread
changes its answer -- and nothing re-asks it, because a thread event starts no
`ci`. A job started under the thread event itself would not help: only a
`pull_request` run's check runs reach the merge box (`gotchas.md`, #131). So
this program finds the head's latest `pull_request`-event `ci` run and re-runs
its `review-clean` JOB through the jobs API. Re-running a job re-runs its
dependents, so `ci-green` re-evaluates in the SAME run, and branch protection
sees the new answer with no push and no hand re-run.

EVERY CASE WHERE NOTHING CAN BE DONE IS A NOTICE AND EXIT 0:
  the run is still in progress    `review-clean` reads live when it runs
  no run exists for this head     nothing to re-evaluate yet
  the API refuses (a fork's pull request has a read-only token)
                                  say so; a person re-runs the job by hand
A failure here would fail a workflow that nobody is watching, on an event
that grants nothing.
"""
from __future__ import annotations

import argparse
import json
import subprocess
import sys

WORKFLOW = "ci.yml"
JOB = "review-clean"


def gh(*args: str) -> tuple[int, str, str]:
    out = subprocess.run(["gh", *args], capture_output=True, text=True)
    return out.returncode, out.stdout, out.stderr


def latest_run(repo: str, sha: str) -> dict | None:
    # `--method GET` IS NOT OPTIONAL: without it `-f` params become a request
    # body and this route answers 404 (`gotchas.md`).
    rc, out, err = gh("api", "--method", "GET", f"repos/{repo}/actions/workflows/{WORKFLOW}/runs",
                      "-f", f"head_sha={sha}", "-f", "event=pull_request", "-f", "per_page=10")
    if rc != 0:
        raise RuntimeError(err.strip() or "listing the runs failed")
    runs = (json.loads(out or "{}").get("workflow_runs") or [])
    if not runs:
        return None
    return max(runs, key=lambda r: r.get("run_started_at") or r.get("created_at") or "")


def job_id(repo: str, run_id: int) -> int | None:
    rc, out, err = gh("api", "--method", "GET", f"repos/{repo}/actions/runs/{run_id}/jobs",
                      "-f", "per_page=100", "--paginate", "--jq", f'.jobs[] | select(.name == "{JOB}") | .id')
    if rc != 0:
        raise RuntimeError(err.strip() or "listing the jobs failed")
    ids = [int(x) for x in out.split() if x.strip().isdigit()]
    return ids[-1] if ids else None


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--sha", required=True, help="the pull request's head commit")
    ap.add_argument("--pr", type=int, required=True, help="the pull request, for the messages")
    args = ap.parse_args()

    try:
        run = latest_run(args.repo, args.sha)
    except (RuntimeError, json.JSONDecodeError) as exc:
        print(f"::notice::#{args.pr}: could not list ci runs for {args.sha[:7]} ({exc}); nothing re-run")
        return 0
    if run is None:
        print(f"::notice::#{args.pr}: no pull_request ci run exists for {args.sha[:7]} yet; nothing to re-evaluate")
        return 0
    if run.get("status") != "completed":
        print(f"#{args.pr}: ci run {run.get('id')} for {args.sha[:7]} is {run.get('status')}; "
              f"{JOB} reads the threads live when it runs, so nothing to do")
        return 0
    try:
        jid = job_id(args.repo, int(run["id"]))
    except (RuntimeError, ValueError, KeyError) as exc:
        print(f"::notice::#{args.pr}: could not list the jobs of run {run.get('id')} ({exc}); nothing re-run")
        return 0
    if jid is None:
        print(f"::notice::#{args.pr}: run {run['id']} has no `{JOB}` job (a draft, or an older workflow); nothing re-run")
        return 0
    rc, _, err = gh("api", "--method", "POST", f"repos/{args.repo}/actions/jobs/{jid}/rerun")
    if rc != 0:
        print(f"::notice::#{args.pr}: re-running `{JOB}` (job {jid} of run {run['id']}) was refused: "
              f"{err.strip() or 'unknown error'}. A fork's pull request has a read-only token here; "
              f"re-run the job by hand.")
        return 0
    print(f"#{args.pr}: re-running `{JOB}` (job {jid}) of ci run {run['id']} on {args.sha[:7]}; "
          f"ci-green re-evaluates with it")
    return 0


if __name__ == "__main__":
    sys.exit(main())
