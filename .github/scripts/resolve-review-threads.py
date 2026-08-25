#!/usr/bin/env python3
"""Resolve the review's OWN threads, and nothing else.

WHY THIS IS A SEPARATE PROGRAM, IN A SEPARATE JOB, WITH NO MODEL IN IT.

GitHub gates `resolveReviewThread` behind repository **Contents: write**, not
Pull requests -- counter-intuitive, since resolving a conversation changes no
file. So the obvious single job would hand a step driven by generated output a
token that can push to this repository.

This project already has the rule, one layer in: *the component running
untrusted model output must not out-rank the one orchestrating it.* The review
job holds `contents: read` and decides WHAT to resolve; this holds
`contents: write` and can only do the one thing, to a list it is handed.

THE AUTHOR CHECK IS HERE AND NOT IN THE PROMPT. Resolving a human reviewer's
thread is the one failure in this whole mechanism that DESTROYS information --
every other mistake adds noise a reader can ignore, while this one hides a
person's objection and reports it as handled. An instruction can be
misinterpreted; a refusal in a program cannot be talked out of.

IT FAILS SAFE ON IDENTITY. A thread is resolved only when its FIRST comment was
written by a login this run recognises as the review's. An unrecognised login is
REFUSED and reported, never resolved -- so the worst outcome of getting the
identity wrong is that nothing is resolved, which is visible, rather than that
somebody's review is closed, which is not.

DETACHMENT IS NOT RESOLUTION, and this program will not be asked to pretend
otherwise: `isOutdated` is reported alongside each refusal so the review can see
what it left behind, but it is never a reason to resolve on its own.
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
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
          comments(first:1) { nodes { author { login } } }
        }
      }
    }
  }
}
"""

RESOLVE_MUTATION = """
mutation($id:ID!) {
  resolveReviewThread(input:{threadId:$id}) { thread { id isResolved } }
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


def fetch_threads(owner: str, repo: str, number: int) -> dict[str, dict]:
    threads: dict[str, dict] = {}
    cursor = None
    while True:
        kwargs = dict(owner=owner, repo=repo, number=number)
        if cursor:
            kwargs["cursor"] = cursor
        page = gh_graphql(THREADS_QUERY, **kwargs)["repository"]["pullRequest"]["reviewThreads"]
        for node in page["nodes"]:
            comments = node["comments"]["nodes"]
            node["author"] = (comments[0]["author"] or {}).get("login") if comments else None
            threads[node["id"]] = node
        if not page["pageInfo"]["hasNextPage"]:
            return threads
        cursor = page["pageInfo"]["endCursor"]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--file", type=pathlib.Path, required=True,
                    help="thread ids the review asked to resolve, one per line")
    ap.add_argument("--authors", default=os.environ.get("REVIEW_AUTHORS", "claude[bot],github-actions[bot]"),
                    help="comma-separated logins the review posts as")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    owner, _, repo = args.repo.partition("/")
    allowed = {a.strip() for a in args.authors.split(",") if a.strip()}

    # AN ABSENT LIST IS NOT AN EMPTY ONE, and reading them alike is what let a
    # severed transfer report the healthy sentence for a whole change. The
    # review writes this file before it uploads it, EVEN WHEN IT HOLDS NOTHING,
    # so a missing one means the list never arrived — not that there was
    # nothing to say.
    if not args.file.is_file():
        print(f"the resolve list is ABSENT at {args.file}: the review's list "
              "never arrived, so nothing could be resolved. An EMPTY list is "
              "the healthy 'nothing to resolve'; a missing one is a broken "
              "transfer.", file=sys.stderr)
        return 1
    wanted = [line.strip() for line in args.file.read_text().splitlines() if line.strip()]
    if not wanted:
        print("nothing to resolve: the review asked for no threads")
        return 0

    threads = fetch_threads(owner, repo, args.pr)

    # Every login that actually authored a thread here. Printed whatever happens:
    # if the identity is wrong, this line is what says so on the first run,
    # rather than a silent no-op that reads like "there was nothing to do".
    seen = sorted({t["author"] for t in threads.values() if t["author"]})
    print(f"thread authors on this pull request: {', '.join(seen) or 'none'}")
    print(f"recognised as the review: {', '.join(sorted(allowed))}")

    resolved = refused = 0
    for thread_id in dict.fromkeys(wanted):          # de-duplicated, order kept
        thread = threads.get(thread_id)
        if thread is None:
            print(f"  REFUSED  {thread_id}: no such thread on this pull request")
            refused += 1
            continue
        if thread["author"] not in allowed:
            print(f"  REFUSED  {thread_id}: authored by {thread['author']!r}, not the review"
                  f"{' (outdated)' if thread['isOutdated'] else ''}")
            refused += 1
            continue
        if thread["isResolved"]:
            print(f"  already  {thread_id} ({thread['path']})")
            continue
        if args.dry_run:
            print(f"  would    {thread_id} ({thread['path']})")
            continue
        gh_graphql(RESOLVE_MUTATION, id=thread_id)
        print(f"  resolved {thread_id} ({thread['path']})")
        resolved += 1

    print(f"\n{resolved} resolved, {refused} refused")
    # A refusal is not a build failure. It is the guard doing its job, and the
    # run's log is where it is reported.
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as exc:
        print(f"gh failed: {exc.stderr or exc}", file=sys.stderr)
        sys.exit(1)
    except RuntimeError as exc:
        print(f"GraphQL error: {exc}", file=sys.stderr)
        sys.exit(1)
