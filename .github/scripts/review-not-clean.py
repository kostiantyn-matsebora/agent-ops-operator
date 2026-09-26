#!/usr/bin/env python3
"""Exit 1 when the review has an open thread of its own on the pull request.

NOT A CHECK, AND NOT A STEP OF THE REVIEW. It was the review's `reconcile`
job's last step, failing the review RUN on an open thread so that `ci-green`
could report the review's content -- and that froze the verdict: a run's
conclusion never changes, so once a person resolved the thread nothing
re-read it, and #220 stayed red with a dispute nobody could answer. Nor can
`ci.yml` ask it: `pull_request_review_thread` is a webhook event and not an
Actions trigger (measured: the workflow file is refused), so no check could
follow a resolution. An open thread blocks the merge through branch
protection's required conversation resolution, evaluated LIVE at merge time,
and that is the one place the content question belongs.

WHAT THIS PROGRAM IS FOR NOW: the conveyor's STATE. `carry.py` asks
it before marking a pull request `loop:mergeable` on a green ci, so the label
is honest at that moment. Carried-over findings, already-open before the
latest review run, count exactly the same as one posted moments ago. A finding folded into a carried thread rather than re-posted
(`review-coordinator.md`: "fold it in, do not post it") is invisible to
`review-post.py`'s own counts for that reason, so the live thread state is the
only correct source, not the run's own tally of what it posted.

THE AUTHOR CHECK IS THE SAME REFUSAL `resolve-review-threads.py` USES, for the
same reason: a human reviewer's own unresolved thread must never fail this
check on the review's behalf. Only a thread whose first comment came from the
review's own recognised login counts.
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys

THREADS_QUERY = """
query($owner:String!, $repo:String!, $number:Int!, $cursor:String) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$number) {
      reviewThreads(first:100, after:$cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isResolved
          isOutdated
          path
          line
          comments(first:1) { nodes { author { login __typename } } }
        }
      }
    }
  }
}
"""


def gh_graphql(query: str, **variables) -> dict:
    cmd = ["gh", "api", "graphql", "-f", f"query={query}"]
    for key, value in variables.items():
        flag = "-F" if isinstance(value, int) else "-f"
        cmd += [flag, f"{key}={value}"]
    out = subprocess.run(cmd, capture_output=True, text=True, check=True).stdout
    payload = json.loads(out)
    if "errors" in payload:
        raise RuntimeError(payload["errors"])
    return payload["data"]


def normalise_login(login: str) -> str:
    """Same normalisation as `resolve-review-threads.py`: REST reports
    `claude[bot]`, GraphQL reports `claude` plus `__typename: Bot`."""
    return login.strip().lower().removesuffix("[bot]")


def fetch_threads(owner: str, repo: str, number: int) -> list[dict]:
    threads: list[dict] = []
    cursor = None
    while True:
        kwargs = dict(owner=owner, repo=repo, number=number)
        if cursor:
            kwargs["cursor"] = cursor
        page = gh_graphql(THREADS_QUERY, **kwargs)["repository"]["pullRequest"]["reviewThreads"]
        threads.extend(page["nodes"])
        if not page["pageInfo"]["hasNextPage"]:
            return threads
        cursor = page["pageInfo"]["endCursor"]


def is_review_authored(thread: dict, allowed: set[str]) -> bool:
    comments = thread["comments"]["nodes"]
    if not comments:
        return False
    author = comments[0].get("author") or {}
    return author.get("__typename") == "Bot" and normalise_login(author.get("login") or "") in allowed


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--authors", default=os.environ.get("REVIEW_AUTHORS", "claude[bot],github-actions[bot]"),
                     help="comma-separated logins the review posts as")
    args = ap.parse_args()
    owner, repo = args.repo.split("/", 1)
    allowed = {normalise_login(a) for a in args.authors.split(",") if a.strip()}

    threads = fetch_threads(owner, repo, args.pr)
    open_findings = [t for t in threads if is_review_authored(t, allowed) and not t["isResolved"]]

    if not open_findings:
        print("no open review-authored threads; clean")
        return 0

    print(f"::error::{len(open_findings)} open review finding(s) left on #{args.pr}:")
    for t in open_findings:
        outdated = " (outdated)" if t.get("isOutdated") else ""
        print(f"  {t['path']}:{t.get('line')}{outdated}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
