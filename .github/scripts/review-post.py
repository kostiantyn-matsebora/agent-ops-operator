#!/usr/bin/env python3
"""Post a review's output to the pull request — every finding, every thread
reply, the resolve list and the one summary — from ONE JSON document, in ONE
call.

The coordinator used to post with one `gh api` per finding, one per reply,
one `mark-thread-resolved.sh` per thread, then the summary — some fifteen
model turns, four of them re-trying a summary whose heredoc to /tmp was
blocked. Posting is not judgement; the judgement is in the JSON this reads.

stdin or a file:
  {"repo": "owner/name", "number": 111,
   "findings": [{"path": "...", "line": 12, "body": "**Claim:** ..."}],
   "replies":  [{"commentId": 123, "body": "Fixed in abc123."}],
   "resolve":  ["PRRT_..."],
   "summary":  "### Review\\n..."}

A finding's `line` must be a line of the diff; the API refuses otherwise,
and the refusal is printed with the finding, never swallowed. The resolve
list goes through `mark-thread-resolved.sh`, which validates the ids and
writes the file `reconcile` reads.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent


def gh(*args: str, stdin: str | None = None) -> tuple[int, str]:
    p = subprocess.run(["gh", *args], capture_output=True, text=True, input=stdin)
    return p.returncode, (p.stdout + p.stderr).strip()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("file", nargs="?", help="the JSON document; stdin when absent")
    args = ap.parse_args()
    text = pathlib.Path(args.file).read_text() if args.file else sys.stdin.read()
    d = json.loads(text)
    repo, number = d["repo"], int(d["number"])

    rc, sha = gh("pr", "view", str(number), "-R", repo, "--json", "headRefOid", "-q", ".headRefOid")
    if rc != 0:
        print(f"::error::could not read the head sha: {sha}", file=sys.stderr)
        return 1

    posted = failed = 0
    for f in d.get("findings", []):
        rc, out = gh("api", f"repos/{repo}/pulls/{number}/comments",
                     "-f", f"commit_id={sha}", "-f", f"path={f['path']}", "-F", f"line={int(f['line'])}",
                     "-f", "side=RIGHT", "-f", f"body={f['body']}")
        if rc == 0:
            posted += 1
        else:
            failed += 1
            print(f"::warning::finding at {f['path']}:{f['line']} not posted: {out[:200]}", file=sys.stderr)

    replied = 0
    for r in d.get("replies", []):
        rc, out = gh("api", f"repos/{repo}/pulls/{number}/comments/{int(r['commentId'])}/replies",
                     "-f", f"body={r['body']}")
        if rc == 0:
            replied += 1
        else:
            print(f"::warning::reply to {r['commentId']} not posted: {out[:200]}", file=sys.stderr)

    recorded = 0
    ids = [i for i in d.get("resolve", []) if i]
    if ids:
        p = subprocess.run([str(HERE / "mark-thread-resolved.sh"), *ids], capture_output=True, text=True)
        if p.returncode == 0:
            recorded = len(ids)
        else:
            print(f"::warning::resolve list not recorded: {(p.stdout + p.stderr).strip()[:200]}", file=sys.stderr)

    summary_posted = False
    if d.get("summary"):
        rc, out = gh("pr", "comment", str(number), "-R", repo, "--body", d["summary"])
        summary_posted = rc == 0
        if not summary_posted:
            print(f"::error::summary not posted: {out[:200]}", file=sys.stderr)

    print(json.dumps({"summaryPosted": summary_posted, "inline": posted, "inlineFailed": failed,
                      "replies": replied, "resolved": recorded}))
    return 0 if summary_posted or not d.get("summary") else 1


if __name__ == "__main__":
    sys.exit(main())
