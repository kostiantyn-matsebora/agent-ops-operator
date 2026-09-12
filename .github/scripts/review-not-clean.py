#!/usr/bin/env python3
"""Fail when the review leaves an open thread of its own on the pull request.

THIS IS THE ONE PLACE "the review ran" AND "the review found nothing to
block" become the same fact a status check can report. `consolidate` already
refuses to report green when the review never posted its summary -- that
guards against the machinery failing silently. It says nothing about the
CONTENT of a review that ran cleanly and posted five open findings, because
until now nothing needed it to: an open thread already blocks merge on its
own, through `required_conversation_resolution`, which is a property of the
pull request GitHub evaluates at merge time and cannot join `ci-green`'s
`needs:` list.

Read AFTER `reconcile` resolves whatever this run's list named -- carried-over
findings, already-open before this run, count exactly the same as one posted
moments ago. A finding folded into a carried thread rather than re-posted
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
