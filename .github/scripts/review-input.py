#!/usr/bin/env python3
"""Build the review's input: what the pull request changed, which of it is
read this run and which is CARRIED from a previous run, grouped into the
components that will each be read, every review thread once, and the
change's delta specs.

NO MODEL. Everything here is deterministic, so it is a program, and the
readings start only once its output exists.

READ UNTIL QUIET, THEN CARRY. The review's own summary comments carry a
hidden coverage marker — the head sha and, per path, how many consecutive
reads added no new finding (`review-post.py` writes it). This program reads
every such marker (newest per path wins), and decides per changed path:
READ from the base, READ from the recorded sha (a delta), or CARRIED —
unread, its standing threads left as they are. See
`openspec/specs/automated-code-review/spec.md`, "A file is read until
independent reads go quiet, then carried".

Writes `review-input.json`:
  {repo, number, base, head, headSha, paths, queue, entries, threads,
   specPaths, since, carried, coverageInvalidated}
and, when $GITHUB_OUTPUT is set, the workflow outputs:
  groups  — the read matrix: [{group, slug, chunk, paths}], one per READ job
  count   — how many jobs
  base    — the base branch name
  head    — the head branch name
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import re
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent

# ONE JOB PER COMPONENT WITH AT LEAST ONE READ PATH. `--chunk N` splits a
# component past N files into several jobs, for an install that wants it or
# for the tests; 0 never splits.
CHUNK = 0

THREADS_QUERY = """
query($o:String!,$r:String!,$n:Int!,$after:String){
  repository(owner:$o,name:$r){pullRequest(number:$n){
    reviewThreads(first:100,after:$after){
      pageInfo{hasNextPage endCursor}
      nodes{id isResolved isOutdated path line
        comments(first:1){nodes{databaseId author{login} body}}}}}}}
"""

COVERAGE_RE = re.compile(r"<!--\s*claude-review-coverage\s+(\{.*?\})\s*-->", re.S)


def sh(*cmd: str) -> str:
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode != 0:
        raise SystemExit(f"{' '.join(cmd[:3])} failed ({p.returncode}): {p.stderr.strip()[:400]}")
    return p.stdout


def git_ok(*cmd: str) -> bool:
    return subprocess.run(["git", *cmd], capture_output=True, text=True).returncode == 0


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


def coverage_marks(repo: str, number: int) -> tuple[dict[str, str], dict[str, int]]:
    """`readAt: path -> sha` and `quietAt: path -> N`, from the newest marker
    naming each path. Comments come back oldest first, so a later one
    overwrites an earlier one — the newest marker wins without sorting by
    time."""
    out = sh("gh", "api", f"repos/{repo}/issues/{number}/comments", "--paginate", "-q", ".[].body")
    read_at: dict[str, str] = {}
    quiet_at: dict[str, int] = {}
    for m in COVERAGE_RE.finditer(out):
        try:
            doc = json.loads(m.group(1))
        except json.JSONDecodeError:
            continue
        sha = doc.get("sha")
        if not isinstance(sha, str):
            continue
        for p, info in (doc.get("paths") or {}).items():
            if not isinstance(info, dict):
                continue
            read_at[p] = sha
            quiet_at[p] = int(info.get("quiet", 0) or 0)
    return read_at, quiet_at


def _diff_touches(sha: str, head: str, *paths: str) -> bool:
    return not git_ok("diff", "--quiet", sha, head, "--", *paths)


def decide(paths: list[str], read_at: dict[str, str], quiet_at: dict[str, int],
           base: str, head: str, full: bool, k: int, spec_dir: str | None) -> tuple[list[str], dict[str, str], list[dict], str]:
    """Per path: READ (with its `since`) or CARRIED. Returns
    (read_paths, since_by_path, carried[{path, since, quiet}], reason)."""
    since: dict[str, str] = {}
    carried: list[dict] = []
    read_paths: list[str] = []
    reason = ""

    invalidate_all = False
    if full:
        invalidate_all, reason = True, "a full review was requested"
    else:
        # Only shas that are still ancestors of head are meaningful here — a
        # sha a rebase left behind is handled per-path below (READ, rebased),
        # and `git diff` against an object that no longer resolves as an
        # ancestor would read as "differs" for the wrong reason.
        shas = sorted({s for s in read_at.values() if s and git_ok("merge-base", "--is-ancestor", s, head)})
        watch = [".claude/rules"] + ([spec_dir] if spec_dir else [])
        if shas and any(_diff_touches(s, head, *watch) for s in shas):
            invalidate_all = True
            reason = "a rule file or the change's delta specs changed since a path was read"

    for p in paths:
        if invalidate_all:
            since[p] = base
            read_paths.append(p)
            print(f"{p}: read (since {base}, {reason})", file=sys.stderr)
            continue
        sha = read_at.get(p)
        if sha is None:
            since[p] = base
            read_paths.append(p)
            print(f"{p}: read (since {base}, new)", file=sys.stderr)
        elif not git_ok("merge-base", "--is-ancestor", sha, head):
            since[p] = base
            read_paths.append(p)
            print(f"{p}: read (since {base}, rebased)", file=sys.stderr)
        elif _diff_touches(sha, head, p):
            since[p] = sha
            read_paths.append(p)
            print(f"{p}: read (since {sha}, changed)", file=sys.stderr)
        else:
            q = quiet_at.get(p, 0)
            if q < k:
                since[p] = sha
                read_paths.append(p)
                print(f"{p}: read (since {sha}, quiet {q} < {k})", file=sys.stderr)
            else:
                carried.append({"path": p, "since": sha, "quiet": q})
                print(f"{p}: carried (quiet {q})", file=sys.stderr)
    return read_paths, since, carried, reason


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--number", required=True, type=int)
    ap.add_argument("--out", type=pathlib.Path, default=pathlib.Path("review-input.json"))
    ap.add_argument("--chunk", type=int, default=CHUNK, help="files per read job")
    ap.add_argument("--full", action="store_true", help="ignore every marker; read every path from the base")
    ap.add_argument("--quiet-reads", type=int, default=1,
                    help="consecutive reads with nothing new before a path is carried")
    args = ap.parse_args()
    chunk = args.chunk if args.chunk > 0 else 10**9

    pr = json.loads(sh("gh", "pr", "view", str(args.number), "-R", args.repo, "--json", "baseRefName,headRefName,headRefOid"))
    base, head, head_sha = pr["baseRefName"], pr["headRefName"], pr.get("headRefOid", "")
    paths = [p for p in sh("gh", "pr", "diff", str(args.number), "-R", args.repo, "--name-only").splitlines() if p.strip()]
    queue = json.loads(sh("python3", str(HERE / "review-queue.py"), *paths)) if paths else []

    read_at, quiet_at = coverage_marks(args.repo, args.number)
    spec_dir = f"openspec/changes/{head[len('change/'):]}/specs" if head.startswith("change/") else None
    read_paths, since, carried, reason = decide(paths, read_at, quiet_at, f"origin/{base}", "HEAD",
                                                 args.full, args.quiet_reads, spec_dir)

    threads_list = threads(args.repo, args.number)
    by_path: dict[str, list[dict]] = {}
    for t in threads_list:
        by_path.setdefault(t["path"], []).append(t)
    for c in carried:
        c["threads"] = by_path.get(c["path"], [])

    read_queue = json.loads(sh("python3", str(HERE / "review-queue.py"), *read_paths)) if read_paths else []

    data = {
        "repo": args.repo, "number": args.number, "base": base, "head": head, "headSha": head_sha,
        "paths": paths, "queue": queue,
        "since": since, "quietAt": quiet_at, "carried": carried, "coverageInvalidated": reason,
        "threads": threads_list,
        "specPaths": spec_paths(head),
    }

    # THE MATRIX ENTRIES: ONE JOB PER COMPONENT WITH A READ PATH. A component
    # whose paths are all carried has no job at all.
    groups = []
    for g in read_queue:
        n = max(1, -(-len(g["paths"]) // chunk))
        for i in range(n):
            chunk_paths = g["paths"][i * chunk:(i + 1) * chunk]
            slug = g["group"].replace("/", "__") + (f"__{i + 1}-of-{n}" if n > 1 else "")
            groups.append({"group": g["group"], "slug": slug,
                           "chunk": f"{i + 1}/{n}" if n > 1 else "", "paths": chunk_paths})
    data["entries"] = groups
    args.out.write_text(json.dumps(data, indent=1) + "\n")
    print(f"{len(queue)} component(s), {len(groups)} job(s), from {len(paths)} path(s) "
          f"({len(read_paths)} read, {len(carried)} carried), "
          f"{len(threads_list)} thread(s), {len(data['specPaths'])} delta spec(s); base {base}, head {head}")
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
