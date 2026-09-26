#!/usr/bin/env python3
"""Who asked for a round, may they, and does one start.

AN ADAPTER, AND THE WORKFLOW'S WHOLE GATE. It maps the event that fired
`review-dispatch.yml` onto the machine's `Trigger`, gathers the facts (the pull
request, who placed its fix label and when, how many rounds ran since, the issue's
labels, and the person behind a carried grant), asks `conveyor.gate`, and does what
it answers: refuse and say why, strip a label, record the loop's and the station's
events, and write the outputs the other jobs read. It replaced about two hundred
lines of shell that held the bot re-check three times, once for each way a bot could
start a round, and a fourth path with none.

INPUTS ARE THE ENVIRONMENT the workflow already passes: EVENT, BODY, ASSOCIATION,
SENDER, PR, REVIEW_TITLE, REVIEW_PR, RUN_HEAD, WORKFLOW_RUN_PATH, INPUT_MODE,
EVENT_LABEL, GITHUB_REPOSITORY. OUTPUTS go to GITHUB_OUTPUT: pr, branch, head,
dispatcher, max_rounds, mode (`all` | `threads` | `none`), approver, since.

WHO PLACED THE FIX LABEL, AND WHEN, comes from the pull request's timeline, which is
state rather than a memory of an event. The latest placement counts, so removing and
re-adding the label starts the round count afresh. A CARRIED LABEL'S TIMELINE ACTOR IS
THE BOT, never the approver, so a bot placement reads the person's login from
`carry.py`'s own marker comment, anchored to its sentence shape rather than "the first @".
"""
from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402
import conveyor_io as io  # noqa: E402

CARRIED = re.compile(r"carrying @([A-Za-z0-9_.-]+)'s standing instruction")


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


def out(name: str, value) -> None:
    io.write_output(name, str(value))


def refuse(repo: str, pr: str, sender: str, reason: str, removed: str = "") -> int:
    tail = f" The `{removed}` label was removed." if removed else ""
    io.comment(repo, int(pr), f"Dispatch by @{sender} refused: {reason}.{tail}")
    print(f"::error::dispatch refused: {reason}")
    return 1


def dispatch_form(vocab: dict, body: str) -> bool:
    """The whole comment, trimmed, trailing punctuation dropped, matched against the vocabulary's forms."""
    tail = vocab.get("trailing_punctuation", ".!,")
    cleaned = (body or "").strip().rstrip(tail).strip().lower()
    return cleaned in {d.lower() for d in vocab["dispatch"]}


def resolve_pr(repo: str) -> str:
    """The pull request a run concerns. A comment, a label or a hand run carries its number. A
    completion carries it in its run's TITLE (`review-dispatch after Review of #<n>`), or its
    payload, or failing both, exactly one open pull request whose head is the run's sha."""
    pr = env("PR")
    if env("EVENT") != "workflow_run":
        return pr
    m = re.search(r"#(\d+)", env("REVIEW_TITLE"))
    pr = m.group(1) if m else env("REVIEW_PR")
    if pr or not env("RUN_HEAD"):
        return pr
    try:
        found = io.gh("api", "--method", "GET", f"repos/{repo}/commits/{env('RUN_HEAD')}/pulls",
                      "--jq", '.[] | select(.state == "open") | .number').split()
    except RuntimeError:
        found = []
    if len(found) == 1:
        return found[0]
    if len(found) > 1:
        print(f"::notice::{env('RUN_HEAD')[:7]} is the head of {len(found)} open pull requests ({' '.join(found)}); "
              "this run names none, so nothing starts")
    return ""


def placement(repo: str, pr: str, fix: str) -> tuple[str, str, bool]:
    """(approver, since, carried) for the pull request's fix label, or ("", "", False)."""
    found = io.label_placement(repo, int(pr), fix)
    if not found:
        return "", "", False
    actor, since = found
    if actor != conveyor.WORKFLOW_BOT:
        return actor, since, False
    try:
        bodies = io.gh("api", f"repos/{repo}/issues/{pr}/comments", "--paginate", "--jq",
                       '[.[] | select(.body | startswith("<!-- carry-grant:fix -->"))] | last | .body')
    except RuntimeError:
        bodies = ""
    m = CARRIED.search(bodies or "")
    return (m.group(1) if m else actor), since, True


def main() -> int:
    repo = env("GITHUB_REPOSITORY")
    vocab = io.vocabulary()
    fix = vocab["approve_label"]
    event, sender_login = env("EVENT"), env("SENDER")
    out("max_rounds", vocab["max_rounds"])
    out("dispatcher", sender_login)

    # ---- the trigger
    if event == "workflow_dispatch":
        # A HAND RUN NEEDS NO FURTHER CHECK: triggering it at all required push access. A
        # BOT-TRIGGERED RUN IS NOT THAT (the open job dispatches this workflow itself), and the
        # machine re-checks it exactly as it re-checks a bot-placed label.
        who = io.placer(repo, sender_login) if sender_login == conveyor.WORKFLOW_BOT else conveyor.Placer(sender_login, "write")
        trigger = conveyor.Trigger("dispatch", who, mode=env("INPUT_MODE") or "threads")
    elif event in ("issue_comment", "pull_request_review_comment"):
        # AUTHORISED BY WHO SENT IT: `author_association` is GitHub's own statement about the author.
        assoc = env("ASSOCIATION")
        who = conveyor.Placer(sender_login, "write" if assoc in ("OWNER", "MEMBER", "COLLABORATOR") else assoc.lower() or "none")
        trigger = conveyor.Trigger("comment", who, comment_is_dispatch=dispatch_form(vocab, env("BODY")))
    elif event == "pull_request":
        trigger = conveyor.Trigger("labeled", io.placer(repo, sender_login), label=env("EVENT_LABEL"))
    elif event == "workflow_run":
        kind = "review_completed" if env("WORKFLOW_RUN_PATH").endswith("claude-review.yml") else "ci_failed"
        trigger = conveyor.Trigger(kind)
    else:
        print(f"::notice::event {event!r} starts nothing here")
        out("mode", "none")
        return 0

    pr = resolve_pr(repo)
    if not pr:
        print(f"::notice::the run '{env('REVIEW_TITLE')}' names no pull request; nothing to do")
        out("mode", "none")
        return 0
    out("pr", pr)

    # ---- the facts
    view = json.loads(io.gh("pr", "view", pr, "--repo", repo, "--json",
                            "isCrossRepository,headRefName,headRefOid,state,body,labels"))
    body = view.get("body") or ""
    m_refs, m_closes = re.search(r"\bRefs #(\d+)", body), re.search(r"\bCloses #(\d+)", body)
    refs, closes = (int(m_refs.group(1)) if m_refs else None), (int(m_closes.group(1)) if m_closes else None)
    labels = frozenset(l.get("name") for l in view.get("labels") or [] if l.get("name"))
    out("branch", view.get("headRefName") or "")
    out("head", view.get("headRefOid") or "")

    approver, since, carried = ("", "", False)
    rounds_used = grants = 0
    if fix in labels:
        approver, since, carried = placement(repo, pr, fix)
        try:
            comments = io.parse_paginated(io.gh("api", f"repos/{repo}/issues/{pr}/comments", "--paginate"))
        except (RuntimeError, json.JSONDecodeError):
            comments = []
        rounds_used = conveyor.count_marked(comments, vocab["round_marker"], since)
        grants = conveyor.count_marked(comments, vocab["grant_marker"], since)
    pull = conveyor.PullRequest(
        state=view.get("state") or "OPEN", labels=labels, refs=refs, closes=closes,
        rounds_used=rounds_used, grants=grants, max_rounds=vocab["max_rounds"],
        fork=bool(view.get("isCrossRepository")))
    trigger = conveyor.Trigger(trigger.event, trigger.sender, trigger.label, trigger.comment_is_dispatch,
                               trigger.mode, label_carried=carried)

    issue = refs or closes
    line = io.line(repo, issue, sessions=False) if issue else conveyor.Line()
    grant = conveyor.standing_grant(vocab, line.issue_labels, "fix", pr_closes=closes is not None) if issue else None
    behind = io.grant_placer(repo, issue, grant) if grant else None

    # ---- the verdict, and what it means
    d = conveyor.gate(vocab, trigger, pull, line, behind)
    print(f"gate for #{pr} ({event}): {d.action}: {d.reason}")

    if event in ("issue_comment", "pull_request_review_comment") and d.action == "none" and not trigger.comment_is_dispatch:
        return refuse(repo, pr, sender_login, "not a dispatch. The form is one of: " + ", ".join(vocab["dispatch"]))
    if d.action == "refuse":
        if d.remove_label:
            subprocess.run(["gh", "pr", "edit", pr, "--repo", repo, "--remove-label", d.remove_label], check=False)
        if d.loop_event:
            io.apply_events(repo, int(pr), loop_event=d.loop_event)
        return refuse(repo, pr, sender_login, d.reason, d.remove_label)
    if d.action == "none":
        if d.loop_event:
            io.apply_events(repo, int(pr), loop_event=d.loop_event)
        # A REVIEW COMPLETED THAT STARTED NO ROUND may still have opened a thread on a label that
        # says mergeable: the refresh sends the loop machine `thread:opened` and the table decides.
        if event == "workflow_run" and trigger.event == "review_completed" and pull.state == "OPEN":
            subprocess.run([sys.executable, ".github/scripts/refresh-loop-state.py", "--repo", repo, "--pr", pr], check=False)
        out("mode", "none")
        return 0
    if d.action == "threads":
        print(f"dispatch on #{pr} ({view.get('headRefName')}) by {sender_login}")
        out("mode", "threads")
        return 0

    # ---- a round starts
    if not approver:
        print(f"::notice::#{pr} carries `{fix}` but the timeline shows nobody placing it; no round starts")
        out("mode", "none")
        return 0
    out("approver", approver)
    out("since", since)
    print(f"round on #{pr} ({view.get('headRefName')} @ {(view.get('headRefOid') or '')[:7]}), "
          f"approved by {approver} at {since}, triggered by {event}")
    io.apply_events(repo, int(pr), loop_event=d.loop_event)
    if issue:
        io.apply_events(repo, issue, station_event=d.station_event)
    out("mode", "all")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except RuntimeError as exc:
        print(f"::error::{exc}", file=sys.stderr)
        sys.exit(1)
