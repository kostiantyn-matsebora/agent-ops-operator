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

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402  -- the state machine: the verdict is its, this program gathers the facts
import conveyor_io as io  # noqa: E402  -- the dispute-thread walk: the sweep reads the same one

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
WORKFLOW = "review-dispatch.yml"


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
    return {"label": doc["approve_label"], "marker": doc["dispute_marker"], "raw": doc}


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
    """What still waits on a person: every unresolved thread carrying an
    unanswered dispute, plus a pull request comment disputing analysis issues
    (a check or a Sonar finding has no thread of its own).

    THE WALK IS `conveyor_io`'S -- shared with the sweep, which reads the exact
    same two-part shape. A `RuntimeError` from that shared read is an
    `Unreadable` here, this program's own fail-open vocabulary."""
    try:
        return io.unanswered_disputes(repo, pr, marker, with_location=True)
    except RuntimeError as exc:
        raise Unreadable(str(exc))


def judge(repo: str, pr: int | None, vocabulary: dict) -> tuple[bool, str]:
    """(allowed, message). Raises Unreadable for anything that fails open.

    THE VERDICT IS THE MACHINE'S (`conveyor.guard`). This gathers the facts: the
    pull request's state and labels, how many fixing rounds are queued or running,
    and how many disputes nobody answered. THE ARCHIVE COMMAND IS THE ONLY CALLER.
    A `ci` purpose used to make this the documentation check's question too, and
    that check was a red no fixer could clear (#259): the open thread already held
    the merge, and the check added nothing but the red. No check asks this now.
    """
    view_args = ["pr", "view"] + ([str(pr)] if pr else []) + \
        ["--repo", repo, "--json", "number,headRefName,labels,state"]
    view = gh_json(*view_args)
    if not view or not view.get("number"):
        raise Unreadable("no pull request to read")
    pr = int(view["number"])
    labels = frozenset(l.get("name") for l in view.get("labels") or [] if l.get("name"))
    facts = conveyor.PullRequest(state=(view.get("state") or "OPEN"), labels=labels)
    vocab = vocabulary["raw"]
    if facts.state != "OPEN" or vocab["approve_label"] not in labels:
        return _verdict(vocab, facts, [], [])

    running = running_rounds(repo, pr, view.get("headRefName") or "")
    disputes = unanswered_disputes(repo, pr, vocabulary["marker"])
    return _verdict(vocab, facts, running, disputes, pr)


def _verdict(vocab: dict, facts, running: list, disputes: list, pr: int = 0) -> tuple[bool, str]:
    d = conveyor.guard(vocab, facts, len(running), len(disputes))
    if d.action == "allow":
        return True, (f"#{pr} carries `{vocab['approve_label']}`, {d.reason}" if pr else d.reason)
    detail = []
    if running:
        detail.append("a fixing round is still running on #%d:\n%s"
                      % (pr, "\n".join(f"  - {r}" for r in running)))
    if disputes:
        detail.append("the fixing step disputed a finding and no person has answered:\n%s"
                      % "\n".join(f"  - {x}" for x in disputes))
    return False, "\n".join(detail)


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
