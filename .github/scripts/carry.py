#!/usr/bin/env python3
"""Carry an issue's standing instruction forward to the next station.

AN ADAPTER. This gathers the facts (the pull request's body and branch, the
issue's labels, who placed the grant and whether they may push, whether the
change is finished, whether a review thread is open), hands them to
`conveyor.carry_fix` or `conveyor.carry_archive`, and does what the decision
says. The decision, including which grant stands for which station, is the
machine's. It replaces `carry-grant.py` and `carry-from-pr.sh`, which held the
same rules twice and in two languages.

A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER MINT ONE. This is
the whole fix for #201: a remote session opened its pull request carrying a label
because its own instructions said to, the gate asked whether the labeller may push,
the labeller was an application, and the label was stripped. The session now labels
nothing, and a WORKFLOW reads the issue's standing instruction again at the moment it
matters, re-checks that whoever placed it can still push, and places the station's
label itself.

  --station fix      after a pull request's CI completes: label the PULL REQUEST
                     `conveyor:fix`, move the loop and station state, and say (through
                     the step output `dispatch_round`) whether to start a round now
  --station archive  after a change's pull request merged: label the ISSUE
                     `conveyor:archive` and say (through `fire_issue`) which issue's
                     archive session to start. Only a FINISHED change reaches this
                     station: a proposal or apply merge carries nothing and moves the
                     line to implement.

ONE COMMENT, ONCE, under a marker naming whose instruction was carried, so a reader
sees whose decision it was on the object the label reached.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import re
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402
import conveyor_io as io  # noqa: E402

MARKER = "<!-- carry-grant:{station} -->"
CHECK_SCRIPT = pathlib.Path(__file__).with_name("review-not-clean.py")


def issue_number(body: str, keyword: str) -> int | None:
    m = re.search(rf"\b{keyword} #(\d+)", body or "")
    return int(m.group(1)) if m else None


def thread_open(repo: str, pr: int) -> bool:
    """A review-authored thread is open, live. Exit 0 is clean, exit 1 is open, and
    anything else is UNKNOWN, which is read as open: claiming green on a read that
    failed would be the worse mistake."""
    if not CHECK_SCRIPT.is_file():
        return True
    rc = subprocess.run([sys.executable, str(CHECK_SCRIPT), "--repo", repo, "--pr", str(pr)],
                        capture_output=True, text=True).returncode
    if rc not in (0, 1):
        print(f"::notice::could not read #{pr}'s review threads (exit {rc}); treated as not mergeable yet")
    return rc != 0


def review_completed(repo: str, head: str) -> bool:
    """The review has run to completion on this head, so its own completion has
    already had its chance to start a round."""
    try:
        raw = io.gh("run", "list", "--repo", repo, "--workflow", "claude-review.yml", "--commit", head,
                    "--json", "status", "--jq", '[.[] | select(.status == "completed")] | length')
        return int(raw or "0") > 0
    except (RuntimeError, ValueError):
        return False


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--station", choices=["fix", "archive"], required=True)
    ap.add_argument("--ci-conclusion", default="", help="--station fix: the CI run's conclusion")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=io.DEFAULT_VOCABULARY)
    args = ap.parse_args()

    vocab = io.vocabulary(args.vocabulary)
    view = json.loads(io.gh("pr", "view", str(args.pr), "--repo", args.repo, "--json",
                            "isCrossRepository,headRefName,headRefOid,body,state,labels"))
    body = view.get("body") or ""
    refs, closes = issue_number(body, "Refs"), issue_number(body, "Closes")
    same = (not view.get("isCrossRepository")) and (view.get("headRefName") or "").startswith("change/")
    labels = frozenset(l.get("name") for l in view.get("labels") or [] if l.get("name"))
    pr = conveyor.PullRequest(
        same_repo_change_branch=same, state=view.get("state") or "OPEN", labels=labels, refs=refs, closes=closes,
        review_completed=args.station == "fix" and review_completed(args.repo, view.get("headRefOid") or ""),
        thread_open=args.station == "fix" and args.ci_conclusion == "success" and thread_open(args.repo, args.pr),
        ci_conclusion=args.ci_conclusion)

    # THE ISSUE THE EVENTS CONCERN is whichever the body names. The GRANT is read from it too,
    # except for the archive station on a pull request that only CLOSES an issue: that merge ends
    # the line and there is nothing left to carry, so no grant is looked up for it.
    issue = refs or closes
    carrying = issue is not None and not (args.station == "archive" and refs is None)
    line = io.line(args.repo, issue, sessions=False) if issue and same else conveyor.Line()

    # THE PERSON BEHIND THE GRANT that stands for THIS pull request and station.
    grant = conveyor.standing_grant(vocab, line.issue_labels, args.station, pr_closes=closes is not None) if carrying else None
    placer = io.grant_placer(args.repo, issue, grant) if grant else None

    decide = conveyor.carry_fix if args.station == "fix" else conveyor.carry_archive
    d = decide(vocab, pr, line, placer)
    print(f"{args.station} carry for #{args.pr}: {d.action}: {d.reason}")

    target = args.pr if args.station == "fix" else issue
    if grant and placer is not None and not placer.may_push and target:
        gone = f"<!-- carry-grant:{args.station}:access-lost -->"
        if not io.already_marked(args.repo, target, gone):
            io.comment(args.repo, target,
                       f"{gone}\n`{grant}` was placed by @{placer.login} on #{issue}, but they now have "
                       f"`{placer.permission}` there. Nothing was carried. Someone with write access can place "
                       f"`{vocab['approve_label'] if args.station == 'fix' else vocab['archive_label']}` directly, "
                       f"or re-place `{grant}`.")

    if d.action == "carry":
        label = vocab["approve_label"] if args.station == "fix" else vocab["archive_label"]
        # THE LABEL IS RE-ASSERTED EVERY TIME, even when the marker comment already
        # stands: `gh ... --add-label` on a present label is a no-op, so a label a
        # person removed comes back on the next transition. The comment, which
        # records whose decision it was, is what is deduplicated.
        io.gh("issue", "edit", str(target), "--repo", args.repo, "--add-label", label)
        where = "pull request" if args.station == "fix" else "issue"
        marker = MARKER.format(station=args.station)
        if not io.already_marked(args.repo, target, marker):
            io.comment(args.repo, target,
                       f"{marker}\nPlaced `{label}` on this {where}, carrying @{placer.login}'s standing "
                       f"instruction (`{d.grant}`, placed on #{issue}). This program relays a grant a person "
                       "already gave; it does not decide anything on its own.")
        print(f"carried `{d.grant}` (from {placer.login}) to `{label}` on {where} #{target}")
        if args.station == "fix":
            io.write_output("dispatch_round", "true" if d.dispatch_round else "false")
        else:
            io.write_output("fire_issue", str(issue))
    elif args.station == "fix":
        io.write_output("dispatch_round", "false")

    # THE MACHINE'S EVENTS, written through the one writer of a state label.
    if d.loop_event:
        io.apply_events(args.repo, args.pr, loop_event=d.loop_event, vocabulary_path=args.vocabulary)
    if d.station_event and issue:
        io.apply_events(args.repo, issue, station_event=d.station_event, vocabulary_path=args.vocabulary)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except RuntimeError as exc:
        print(f"::error::{exc}", file=sys.stderr)
        sys.exit(1)
