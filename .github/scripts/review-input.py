#!/usr/bin/env python3
"""Build the review's input: what the pull request changed, grouped into the
components that will each be read, every review thread once, and the change's
delta specs.

NO MODEL. This used to be an agent asked to run three commands; it took 136
seconds and eleven refused commands on one pull request to do what this does
in two. Everything here is deterministic, so it is a program, and the readings
start only once its output exists.

Writes `review-input.json`:
  {repo, number, base, head, paths, queue, threads, specPaths}
and, when $GITHUB_OUTPUT is set, the workflow outputs the jobs key on:
  groups  — the read matrix: [{group, slug}], slug being the artifact-safe name
  count   — how many
  base    — the base branch name
  head    — the head branch name
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent

THREADS_QUERY = """
query($o:String!,$r:String!,$n:Int!){
  repository(owner:$o,name:$r){pullRequest(number:$n){
    reviewThreads(first:100){nodes{id isResolved isOutdated path line
      comments(first:1){nodes{databaseId author{login} body}}}}}}}
"""


def sh(*cmd: str) -> str:
    return subprocess.run(cmd, capture_output=True, text=True, check=True).stdout


def threads(repo: str, number: int) -> list[dict]:
    owner, name = repo.split("/", 1)
    out = sh("gh", "api", "graphql", "-f", f"query={THREADS_QUERY}",
             "-f", f"o={owner}", "-f", f"r={name}", "-F", f"n={number}")
    nodes = json.loads(out)["data"]["repository"]["pullRequest"]["reviewThreads"]["nodes"]
    flat = []
    for t in nodes:
        first = (t.get("comments") or {}).get("nodes") or [{}]
        c = first[0] if first else {}
        flat.append({
            "id": t["id"], "path": t.get("path") or "", "line": t.get("line"),
            "isResolved": bool(t.get("isResolved")), "isOutdated": bool(t.get("isOutdated")),
            "commentId": c.get("databaseId"),
            "author": ((c.get("author") or {}).get("login")) or "",
            "body": c.get("body") or "",
        })
    return flat


def spec_paths(head: str) -> list[str]:
    if not head.startswith("change/"):
        return []
    name = head[len("change/"):]
    out = sh("git", "ls-files", "--", f"openspec/changes/{name}/specs/")
    return [p for p in out.splitlines() if p]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--number", required=True, type=int)
    ap.add_argument("--out", type=pathlib.Path, default=pathlib.Path("review-input.json"))
    args = ap.parse_args()

    pr = json.loads(sh("gh", "pr", "view", str(args.number), "--json", "baseRefName,headRefName"))
    base, head = pr["baseRefName"], pr["headRefName"]
    paths = [p for p in sh("gh", "pr", "diff", str(args.number), "--name-only").splitlines() if p.strip()]
    queue = json.loads(sh("python3", str(HERE / "review-queue.py"), *paths)) if paths else []

    data = {
        "repo": args.repo, "number": args.number, "base": base, "head": head,
        "paths": paths, "queue": queue,
        "threads": threads(args.repo, args.number),
        "specPaths": spec_paths(head),
    }
    args.out.write_text(json.dumps(data, indent=1) + "\n")

    # THE MATRIX ENTRIES. `slug` is the group with `/` replaced, because an
    # artifact name may not carry a slash and a matrix job needs one per
    # reading; the reading itself names its component inside.
    groups = [{"group": g["group"], "slug": g["group"].replace("/", "__")} for g in queue]
    print(f"{len(groups)} component(s) from {len(paths)} path(s), {len(data['threads'])} thread(s), "
          f"{len(data['specPaths'])} delta spec(s); base {base}, head {head}")
    gh_out = os.environ.get("GITHUB_OUTPUT")
    if gh_out:
        with open(gh_out, "a") as f:
            f.write(f"groups={json.dumps(groups)}\n")
            f.write(f"count={len(groups)}\n")
            f.write(f"base={base}\n")
            f.write(f"head={head}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
