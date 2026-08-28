#!/usr/bin/env python3
"""Build the review's input: what the pull request changed, grouped into the
components that will each be read, every review thread once, and the change's
delta specs.

NO MODEL. This used to be an agent asked to run three commands; it took 136
seconds and eleven refused commands on one pull request to do what this does
in two. Everything here is deterministic, so it is a program, and the readings
start only once its output exists.

Writes `review-input.json`:
  {repo, number, base, head, paths, queue, entries, threads, specPaths}
and, when $GITHUB_OUTPUT is set, the workflow outputs the jobs key on:
  groups  — the read matrix: [{group, slug, chunk, paths}], one per job of up
            to CHUNK files; slug is the artifact-safe name
  count   — how many jobs
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

# ONE JOB PER COMPONENT. The split is at the subagent level — workers over a
# queue of files inside the job (`review-component.js`) — because a job costs
# ~30-40 s of checkout, cache and session before it reads anything. `--chunk
# N` splits a component past N files into several jobs, for an install that
# wants it or for the tests; 0 never splits.
CHUNK = 0

THREADS_QUERY = """
query($o:String!,$r:String!,$n:Int!,$after:String){
  repository(owner:$o,name:$r){pullRequest(number:$n){
    reviewThreads(first:100,after:$after){
      pageInfo{hasNextPage endCursor}
      nodes{id isResolved isOutdated path line
        comments(first:1){nodes{databaseId author{login} body}}}}}}}
"""


def sh(*cmd: str) -> str:
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode != 0:
        raise SystemExit(f"{' '.join(cmd[:3])} failed ({p.returncode}): {p.stderr.strip()[:400]}")
    return p.stdout


def threads(repo: str, number: int) -> list[dict]:
    """EVERY thread, paginated. A page of 100 is more than a review leaves,
    until the pull request that proves otherwise — and a thread the review
    never sees is one it raises again."""
    owner, name = repo.split("/", 1)
    nodes: list[dict] = []
    after: str | None = None
    while True:
        args = ["gh", "api", "graphql", "-f", f"query={THREADS_QUERY}",
                "-f", f"o={owner}", "-f", f"r={name}", "-F", f"n={number}"]
        if after:
            args += ["-f", f"after={after}"]
        page = json.loads(sh(*args))["data"]["repository"]["pullRequest"]["reviewThreads"]
        nodes.extend(page.get("nodes") or [])
        info = page.get("pageInfo") or {}
        if not info.get("hasNextPage"):
            break
        after = info["endCursor"]
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
    ap.add_argument("--chunk", type=int, default=CHUNK, help="files per read job")
    args = ap.parse_args()
    chunk = args.chunk if args.chunk > 0 else 10**9

    pr = json.loads(sh("gh", "pr", "view", str(args.number), "-R", args.repo, "--json", "baseRefName,headRefName,headRefOid"))
    base, head, head_sha = pr["baseRefName"], pr["headRefName"], pr.get("headRefOid", "")
    paths = [p for p in sh("gh", "pr", "diff", str(args.number), "-R", args.repo, "--name-only").splitlines() if p.strip()]
    queue = json.loads(sh("python3", str(HERE / "review-queue.py"), *paths)) if paths else []

    data = {
        "repo": args.repo, "number": args.number, "base": base, "head": head, "headSha": head_sha,
        "paths": paths, "queue": queue,
        "threads": threads(args.repo, args.number),
        "specPaths": spec_paths(head),
    }

    # THE MATRIX ENTRIES: ONE JOB PER COMPONENT. Splitting past `--chunk`
    # files exists for an install that wants it and for the tests; the
    # coordinator's input merges a component's chunks back into one reading.
    # `slug` is the artifact-safe name (no slash; the chunk index when split).
    groups = []
    for g in queue:
        n = max(1, -(-len(g["paths"]) // chunk))
        for i in range(n):
            chunk_paths = g["paths"][i * chunk:(i + 1) * chunk]
            slug = g["group"].replace("/", "__") + (f"__{i + 1}-of-{n}" if n > 1 else "")
            groups.append({"group": g["group"], "slug": slug,
                           "chunk": f"{i + 1}/{n}" if n > 1 else "", "paths": chunk_paths})
    data["entries"] = groups
    args.out.write_text(json.dumps(data, indent=1) + "\n")
    print(f"{len(queue)} component(s), {len(groups)} job(s), from {len(paths)} path(s), "
          f"{len(data['threads'])} thread(s), {len(data['specPaths'])} delta spec(s); base {base}, head {head}")
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
