#!/usr/bin/env python3
"""Post a review's output to the pull request — every finding, every thread
reply, the resolve list and the one summary — from ONE JSON document, in ONE
call.

The coordinator used to post with one `gh api` per finding, one per reply,
one `mark-thread-resolved.sh` per thread, then the summary — some fifteen
model turns, four of them re-trying a summary whose heredoc to /tmp was
blocked. Posting is not judgement; the judgement is in the JSON this reads.

stdin or a file (`repo` and `number` may be omitted: they are read from
`review-input.json` beside the file, in $GITHUB_WORKSPACE, or in the working
directory, and last from $GITHUB_REPOSITORY):
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
import os
import pathlib
import subprocess
import sys
import time

HERE = pathlib.Path(__file__).resolve().parent


def gh(*args: str, stdin: str | None = None) -> tuple[int, str]:
    """One gh call, retried on GitHub's SECONDARY rate limit.

    A review posts dozens of comments in a burst, and after a day of review
    runs on one pull request GitHub answered every reply with
    `{"code":"abuse"}` — nothing was posted, and the gate failed a review that
    had run. That limit is the abuse detector asking for a pause, so a pause
    is what it gets: a short one between every post, and a growing one on the
    refusal itself, three times before giving up.
    """
    for attempt in range(4):
        p = subprocess.run(["gh", *args], capture_output=True, text=True, input=stdin)
        out = (p.stdout + p.stderr).strip()
        limited = p.returncode != 0 and ('"code":"abuse"' in out or "secondary rate limit" in out.lower())
        if not limited or attempt == 3:
            time.sleep(1)  # the pause between posts, whatever the answer
            return p.returncode, out
        wait = 30 * (2 ** attempt)
        print(f"::warning::secondary rate limit; waiting {wait}s before retrying", file=sys.stderr)
        time.sleep(wait)
    return p.returncode, out


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("file", nargs="?", help="the JSON document; stdin when absent")
    args = ap.parse_args()
    text = pathlib.Path(args.file).read_text() if args.file else sys.stdin.read()
    d = json.loads(text)
    # THE ADDRESS IS THE WORKFLOW'S FACT, NOT THE MODEL'S. The schema asks the
    # coordinator for `repo` and `number`, and one run answered without them —
    # a thin answer, two tool calls — so this crashed with KeyError, posted
    # nothing, and the gate failed a review that had actually run. The input
    # document the job built already names both; the model's copy is only
    # ever a restatement of it, so it is read second.
    # The input document lives where the job wrote it: beside the posting
    # document when one was given as a file, else the workflow's workspace,
    # else the working directory. Named here so nothing has to guess.
    here = pathlib.Path(args.file).resolve().parent if args.file else pathlib.Path.cwd()
    fallback: dict = {}
    for base in (here, pathlib.Path(os.environ.get("GITHUB_WORKSPACE", "")), pathlib.Path.cwd()):
        address = base / "review-input.json"
        if str(base) and address.is_file():
            fallback = json.loads(address.read_text())
            break
    repo = d.get("repo") or fallback.get("repo") or os.environ.get("GITHUB_REPOSITORY", "")
    number = d.get("number") or fallback.get("number")
    if not repo or number is None:
        print("::error::the posting document names no pull request, and review-input.json is absent", file=sys.stderr)
        return 1
    number = int(number)

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
