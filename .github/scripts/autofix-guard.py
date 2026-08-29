#!/usr/bin/env python3
"""Refuse to archive a change while its automatic fixing loop is open.

`openspec archive` folds a change's deltas into the published specs -- the
point of no return. A pull request labelled for automatic fixing may still be
mid-round (a commit about to land on it) or may carry a DISPUTE the fixing
step posted and no person has answered. Archiving under either records the
change as finished while the pull request cannot merge.

TWO REFUSALS, both read from the pull request and nothing else:

  1. A `review-dispatch` run for this pull request is queued or in progress.
  2. A review thread, or a pull request comment, carries the dispute marker
     and no comment by a PERSON follows it.

FAILS OPEN on everything it cannot read. No `gh`, no pull request for the
branch, no label, an API error: the archive proceeds, and the reason is
printed. A guard that blocks work it does not understand gets disabled, and
then it enforces nothing -- the same argument `docs-task-guard.py` makes,
and this program runs beside it in the same hook and the same CI job.

The label and the marker come from `.github/review-triage.json`, so the
vocabulary is stated once.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import shutil
import subprocess
import sys

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
WORKFLOW = "review-dispatch.yml"

THREADS_QUERY = """
query($owner:String!, $repo:String!, $number:Int!, $cursor:String) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$number) {
      reviewThreads(first:50, after:$cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isResolved
          path
          line
          comments(first:100) {
            nodes { body author { login __typename } }
          }
        }
      }
    }
  }
}
"""


class Unreadable(Exception):
    """Something the guard cannot read. Always fail-open."""


def gh(*args: str) -> str:
    try:
        return subprocess.run(["gh", *args], capture_output=True, text=True, check=True).stdout
    except subprocess.CalledProcessError as exc:
        raise Unreadable(f"gh {args[0]} {args[1] if len(args) > 1 else ''}: {(exc.stderr or '').strip() or exc}")


def gh_json(*args: str):
    out = gh(*args)
    try:
        return json.loads(out or "null")
    except json.JSONDecodeError as exc:
        raise Unreadable(f"unreadable JSON from gh {args[0]}: {exc}")


def load_vocabulary(path: pathlib.Path) -> dict:
    doc = json.loads(path.read_text())
    return {"label": doc["approve_label"], "marker": doc["dispute_marker"]}


def is_person(author: dict | None) -> bool:
    return bool(author) and author.get("__typename") != "Bot"


def unanswered_after_marker(comments: list[dict], marker: str) -> bool:
    """True when a comment carries the marker and no PERSON commented after it.
    Pure, so the suite exercises it without a network."""
    disputed_at = None
    for i, c in enumerate(comments):
        if marker in (c.get("body") or ""):
            disputed_at = i
    if disputed_at is None:
        return False
    return not any(is_person(c.get("author")) for c in comments[disputed_at + 1:])


def running_rounds(repo: str, pr: int, branch: str) -> list[str]:
    """Runs of the dispatch workflow that are not finished and concern this
    pull request -- by head branch (a comment or label event) or by the
    `#<n>` the run name carries (a review-completion event runs on the
    default branch and names the pull request in its title instead)."""
    found = []
    for status in ("queued", "in_progress", "waiting", "requested", "pending"):
        runs = gh_json("run", "list", "--repo", repo, "--workflow", WORKFLOW, "--status", status,
                       "--json", "databaseId,displayTitle,headBranch,url") or []
        for run in runs:
            title = run.get("displayTitle") or ""
            if run.get("headBranch") == branch or f"#{pr}" in title.split() or title.endswith(f"#{pr}"):
                found.append(f"{run.get('url') or run.get('databaseId')} ({status}: {title})")
    return found


def unanswered_disputes(repo: str, pr: int, marker: str) -> list[str]:
    owner, _, name = repo.partition("/")
    found = []
    cursor = None
    while True:
        cmd = ["api", "graphql", "-f", f"query={THREADS_QUERY}", "-f", f"owner={owner}",
               "-f", f"repo={name}", "-F", f"number={pr}"]
        if cursor:
            cmd += ["-f", f"cursor={cursor}"]
        data = gh_json(*cmd)
        try:
            page = data["data"]["repository"]["pullRequest"]["reviewThreads"]
        except (KeyError, TypeError):
            raise Unreadable("the thread query returned no pull request")
        for t in page["nodes"]:
            if t.get("isResolved"):
                continue
            if unanswered_after_marker(t["comments"]["nodes"], marker):
                found.append(f"thread {t['id']} ({t.get('path')}:{t.get('line') or '?'})")
        if not page["pageInfo"]["hasNextPage"]:
            break
        cursor = page["pageInfo"]["endCursor"]

    # A disputed ANALYSIS issue has no thread; the dispute is a pull request
    # comment, answered by a later comment from a person.
    comments = gh_json("api", f"repos/{repo}/issues/{pr}/comments", "--paginate") or []
    shaped = [{"body": c.get("body"),
               "author": {"login": (c.get("user") or {}).get("login"),
                          "__typename": (c.get("user") or {}).get("type")}} for c in comments]
    if unanswered_after_marker(shaped, marker):
        found.append("a pull request comment disputing analysis issues")
    return found


def judge(repo: str, pr: int | None, vocabulary: dict) -> tuple[bool, str]:
    """(allowed, message). Raises Unreadable for anything that fails open."""
    view_args = ["pr", "view"] + ([str(pr)] if pr else []) + \
        ["--repo", repo, "--json", "number,headRefName,labels,state"]
    view = gh_json(*view_args)
    if not view or not view.get("number"):
        raise Unreadable("no pull request to read")
    pr = int(view["number"])
    if view.get("state") and view["state"] != "OPEN":
        return True, f"#{pr} is {view['state'].lower()}; no loop can run on it"
    labels = {l.get("name") for l in view.get("labels") or []}
    if vocabulary["label"] not in labels:
        return True, f"#{pr} does not carry `{vocabulary['label']}`; nothing to wait for"

    reasons = []
    running = running_rounds(repo, pr, view.get("headRefName") or "")
    if running:
        reasons.append("a fixing round is still running on #%d:\n%s"
                       % (pr, "\n".join(f"  - {r}" for r in running)))
    disputes = unanswered_disputes(repo, pr, vocabulary["marker"])
    if disputes:
        reasons.append("the fixing step disputed a finding and no person has answered:\n%s"
                       % "\n".join(f"  - {d}" for d in disputes))
    if reasons:
        return False, "\n".join(reasons)
    return True, f"#{pr} carries `{vocabulary['label']}`, no round is running and every dispute is answered"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", help="owner/name; default: the checkout's")
    ap.add_argument("--pr", type=int, help="the pull request; default: the current branch's")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    args = ap.parse_args()

    def allow(why: str) -> int:
        print(f"autofix-guard: allowed — {why}")
        return 0

    if shutil.which("gh") is None:
        return allow("no gh on PATH, so nothing can be read (fail-open)")
    try:
        vocabulary = load_vocabulary(args.vocabulary)
    except (OSError, KeyError, json.JSONDecodeError) as exc:
        return allow(f"no vocabulary to read ({exc}); fail-open")
    try:
        repo = args.repo or gh_json("repo", "view", "--json", "nameWithOwner")["nameWithOwner"]
        ok, message = judge(repo, args.pr, vocabulary)
    except Unreadable as exc:
        return allow(f"{exc} (fail-open)")
    except (KeyError, TypeError, ValueError) as exc:
        return allow(f"unexpected shape from gh ({exc}); fail-open")
    if ok:
        return allow(message)
    print(f"autofix-guard: REFUSED —\n{message}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
