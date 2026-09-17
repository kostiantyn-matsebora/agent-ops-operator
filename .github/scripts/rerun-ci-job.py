#!/usr/bin/env python3
"""Re-run one FAILED job of a pull request head's own `ci` run.

WHY A RE-RUN, AND NOT A RUN OF ITS OWN. Some checks read the CONVERSATION, not
the tree: `docs-task` asks `autofix-guard.py` whether a dispute the fixing loop
posted has been answered by a person. A reply changes that answer, and nothing
re-asks it -- a comment starts no `ci`. A job started under the comment event
itself would not help: only a `pull_request` run's check runs reach the merge
box (`gotchas.md`, #131). So this program finds the head's latest
`pull_request`-event `ci` run and re-runs the NAMED job through the jobs API,
only if that job failed. Re-running a job re-runs its dependents, so
`ci-green` re-evaluates in the SAME run, and branch protection sees the new
answer with no push and no hand re-run -- which is what the conveyor is for.

EVERY CASE WHERE NOTHING CAN BE DONE IS A NOTICE AND EXIT 0:
  the run is still in progress    the job reads the conversation when it runs
  no run exists for this head     nothing to re-evaluate yet
  the job did not fail            nothing to re-evaluate
  the API refuses (a fork's pull request has a read-only token)
                                  say so; a person re-runs the job by hand
A failure here would fail a workflow nobody is watching, on an event that
grants nothing.
"""
from __future__ import annotations

import argparse
import json
import subprocess
import sys

WORKFLOW = "ci.yml"


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


def parse_paginated(raw: str) -> list[dict]:
    """`gh api --paginate` without `--jq` concatenates each page's JSON object
    back to back; split them the way carry-grant.py splits arrays."""
    try:
        return [json.loads(raw or "{}")]
    except json.JSONDecodeError:
        pages: list[dict] = []
        for chunk in raw.replace("}{", "}\n{").splitlines():
            if chunk.strip():
                pages.append(json.loads(chunk))
        return pages


def failed_job(repo: str, run_id: int, name: str) -> tuple[int | None, str]:
    """(job id, its conclusion) for the LATEST attempt of the named job. The
    name is matched HERE, in Python, never interpolated into a jq program: a
    quote in it would break the filter, and `gh api` has no `--arg`."""
    rc, out, err = gh("api", "--method", "GET", f"repos/{repo}/actions/runs/{run_id}/jobs",
                      "-f", "per_page=100", "--paginate")
    if rc != 0:
        raise RuntimeError(err.strip() or "listing the jobs failed")
    jobs = [j for page in parse_paginated(out) for j in (page.get("jobs") or []) if j.get("name") == name]
    if not jobs:
        return None, "absent"
    last = jobs[-1]
    return int(last["id"]), (last.get("conclusion") or "")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--sha", required=True, help="the pull request's head commit")
    ap.add_argument("--pr", type=int, required=True, help="the pull request, for the messages")
    ap.add_argument("--job", required=True, help="the ci job to re-run, by its name in ci.yml")
    args = ap.parse_args()
    job = args.job

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
              f"`{job}` reads the conversation when it runs, so nothing to do")
        return 0
    try:
        jid, conclusion = failed_job(args.repo, int(run["id"]), job)
    except (RuntimeError, ValueError, KeyError) as exc:
        print(f"::notice::#{args.pr}: could not list the jobs of run {run.get('id')} ({exc}); nothing re-run")
        return 0
    if jid is None:
        print(f"::notice::#{args.pr}: run {run['id']} has no `{job}` job; nothing re-run")
        return 0
    if conclusion != "failure":
        print(f"#{args.pr}: `{job}` concluded {conclusion or 'nothing'} on run {run['id']}; nothing to re-evaluate")
        return 0
    rc, _, err = gh("api", "--method", "POST", f"repos/{args.repo}/actions/jobs/{jid}/rerun")
    if rc != 0:
        print(f"::notice::#{args.pr}: re-running `{job}` (job {jid} of run {run['id']}) was refused: "
              f"{err.strip() or 'unknown error'}. A fork's pull request has a read-only token here; "
              f"re-run the job by hand.")
        return 0
    print(f"#{args.pr}: re-running `{job}` (job {jid}) of ci run {run['id']} on {args.sha[:7]}; "
          f"ci-green re-evaluates with it")
    return 0


if __name__ == "__main__":
    sys.exit(main())
