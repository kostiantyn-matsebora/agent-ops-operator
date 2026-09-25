#!/usr/bin/env python3
"""A label on an issue starts one remote session that implements it.

THE HOP EXISTS TO BUY A GATE AND A RECORD. A cloud routine's own GitHub
triggers are `pull_request` and `release`; there is no `issues` event, so a
routine cannot subscribe to a label directly. Going through an Actions workflow
is not merely the available path -- it is the one that can ask WHO placed the
label, and the one that can leave the session's link on the issue.

  the gate    GitHub lets anyone with TRIAGE label an issue, and triage is not
              write. A routine's own filters read labels, never who placed
              them. This asks the collaborators API, exactly as the fixing
              loop's label gate does, because a label event carries no
              `author_association`. Refused, the label is REMOVED with a
              comment: a refusal that left it in place reads as a broken bot.

  the record  the fire returns a session url, and it is posted on the issue
              ONCE, under a marker. That comment is the transition record for
              the start; the promotion the session then performs leaves its own
              pointer, and nothing else automated is added to the issue.

THE PAYLOAD IS THE ISSUE'S NUMBER AND NOTHING ELSE. The platform wraps fire
text in a block labelled untrusted, for exactly the case where the token leaks.
A number is something the routine's prompt can validate before it is used; the
session then reads the issue itself, through `gh`, under the proxy. Sending the
title and body would put a stranger's prose where the prompt is.

THE ENDPOINT AND ITS TOKEN ARE NOT IN THE TREE. `ROUTINE_FIRE_URL` is a
repository variable and `ROUTINE_FIRE_TOKEN` a repository secret: the url
carries the routine's id, which is a value to rotate when the routine is
recreated, for no reader's benefit.
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import subprocess
import sys
import urllib.error
import urllib.request

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402  -- the state machine: which station a label starts is its rule
import conveyor_io as io  # noqa: E402  -- the facts, gathered once for every adapter

MARKER = "<!-- remote-implement:fired -->"
# PER STATION. The implement marker keeps its original text so every issue
# already carrying it still reads as fired; the archive station records its
# own, or the archive would never fire on an issue the implement did.
STATION_MARKER = {"implement": MARKER, "archive": "<!-- remote-implement:fired:archive -->"}
BETA = "experimental-cc-routine-2026-04-01"
API_VERSION = "2023-06-01"


def gh(*args: str, check: bool = True) -> str:
    out = subprocess.run(["gh", *args], capture_output=True, text=True)
    if check and out.returncode != 0:
        raise RuntimeError(f"gh {' '.join(args)}: {out.stderr.strip()}")
    return out.stdout.strip()


def vocabulary(path: pathlib.Path) -> dict:
    return json.loads(path.read_text())


def comment(repo: str, number: int, body: str) -> None:
    subprocess.run(["gh", "issue", "comment", str(number), "--repo", repo, "--body", body],
                   check=False)


def fire(url: str, token: str, number: int, timeout: int = 30) -> dict:
    """POST the number. Anything but 2xx is raised, and the caller says so on
    the issue -- a silent no-op reads as a broken bot."""
    req = urllib.request.Request(
        url,
        data=json.dumps({"text": str(number)}).encode(),
        headers={
            "content-type": "application/json",
            "authorization": f"Bearer {token}",
            "anthropic-beta": BETA,
            "anthropic-version": API_VERSION,
        },
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = resp.read().decode() or "{}"
    try:
        return json.loads(body)
    except json.JSONDecodeError:
        return {}


def session_url(payload: dict) -> str:
    """The platform names it differently across versions, so read the ones it
    has used and fall back to saying a session started without a link."""
    for key in ("session_url", "sessionUrl", "url", "html_url"):
        value = payload.get(key)
        if value:
            return str(value)
    run = payload.get("run") or payload.get("session") or {}
    if isinstance(run, dict):
        for key in ("session_url", "sessionUrl", "url", "html_url", "id"):
            value = run.get(key)
            if value:
                return str(value)
    return ""


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--event", type=pathlib.Path,
                    default=pathlib.Path(os.environ.get("GITHUB_EVENT_PATH", "")),
                    help="the issues.labeled event payload")
    ap.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", ""))
    ap.add_argument("--vocabulary", type=pathlib.Path,
                    default=pathlib.Path(__file__).resolve().parents[1] / "review-triage.json")
    ap.add_argument("--fire-url", default=os.environ.get("ROUTINE_FIRE_URL", ""),
                    help="the routine's fire endpoint; the suite points this at a local stub")
    args = ap.parse_args()
    if not args.event or not args.event.is_file():
        print("no event payload; nothing to do", file=sys.stderr)
        return 0
    event = json.loads(args.event.read_text())
    label = (event.get("label") or {}).get("name") or ""
    vocab = io.vocabulary(args.vocabulary)
    issue = event.get("issue") or {}
    number = issue.get("number")
    known = {vocab["implement_label"], vocab["run_label"], vocab["archive_label"]}
    if label not in known:
        print(f"label {label!r} is not in {sorted(known)!r}; nothing to do")
        return 0
    if not isinstance(number, int):
        print(f"::error::the event names no issue number ({number!r})")
        return 1
    if issue.get("pull_request"):
        print(f"::notice::#{number} is a pull request, not an issue; nothing fires")
        return 0
    sender = (event.get("sender") or {}).get("login") or ""

    # THE FACTS, then the MACHINE'S verdict. Which station a label starts, whether a
    # carried placement is still backed by a person who can push, whether a station
    # already fired and whether a session is still at work are all `conveyor.fire`'s
    # rule. This program used to hold them, and held them slightly differently from
    # the carry and the gate.
    who = io.placer(args.repo, sender)
    behind = io.grant_placer(args.repo, number, vocab["run_label"]) if who.bot else None
    line = io.line(args.repo, number)
    d = conveyor.fire(vocab, label, line, who, behind)

    if d.action == "refuse":
        if d.remove_label:
            subprocess.run(["gh", "issue", "edit", str(number), "--repo", args.repo,
                            "--remove-label", d.remove_label], check=False)
        comment(args.repo, number,
                f"`{label}` was placed {'by the workflow' if who.bot else f'by @{sender}'}, and it starts nothing: "
                f"{d.reason}. "
                + (f"The label was removed. Someone with write access can place `{label}` directly, "
                   f"or re-place `{vocab['run_label']}`." if d.remove_label else
                   "The label stays, and the line waits for the change to be finished."))
        print(f"::error::{sender or 'the workflow'} placed {label}: {d.reason}"
              + ("; the label was removed" if d.remove_label else ""))
        return 1
    if d.action == "skip":
        print(f"::notice::#{number}: {d.reason}; not firing again")
        return 0
    station = d.station
    carried_by = behind.login if who.bot and behind else ""

    fire_url = (args.fire_url or "").strip()
    token = os.environ.get("ROUTINE_FIRE_TOKEN", "").strip()
    if fire_url and (any(c in fire_url for c in "\r\n\t ")
                     or not fire_url.lower().startswith(("http://", "https://"))):
        comment(args.repo, number,
                f"`{label}` was placed by @{sender}, but this repository's `ROUTINE_FIRE_URL` "
                "is not a URL. Nothing started; check the variable and place the label again.")
        print(f"::error::ROUTINE_FIRE_URL is not a URL: {fire_url[:60]!r}")
        return 1
    if any(c in token for c in "\r\n\t "):
        comment(args.repo, number,
                f"`{label}` was placed by @{sender}, but this repository's "
                "`ROUTINE_FIRE_TOKEN` contains a line break or space — it was probably "
                "copied out of a wrapped display. Nothing started; set it again and "
                "place the label back.")
        print("::error::ROUTINE_FIRE_TOKEN contains whitespace; refusing to send it")
        return 1
    if not fire_url or not token:
        comment(args.repo, number,
                f"`{label}` was placed by @{sender}, but this repository has no routine "
                "configured (`ROUTINE_FIRE_URL` / `ROUTINE_FIRE_TOKEN`). Nothing started.")
        print("::error::ROUTINE_FIRE_URL or ROUTINE_FIRE_TOKEN is not set")
        return 1
    try:
        payload = fire(fire_url, token, number)
    except urllib.error.HTTPError as exc:
        comment(args.repo, number,
                f"Starting a session for this issue failed: the routine's endpoint answered "
                f"`{exc.code} {exc.reason}`. Nothing started; @{sender} can place the label "
                "again once it is fixed.")
        print(f"::error::fire failed: {exc.code} {exc.reason}")
        return 1
    except (urllib.error.URLError, TimeoutError, ValueError, OSError) as exc:
        comment(args.repo, number,
                f"Starting a session for this issue failed: the routine's endpoint could not "
                f"be reached (`{exc}`). Nothing started.")
        print(f"::error::fire failed: {exc}")
        return 1
    url = session_url(payload)
    where = f"[session]({url})" if url else "the session"
    approver = f"@{carried_by}'s standing instruction, carried by the workflow" if carried_by else f"@{sender}"
    # THE STATION'S EVENT, through the one writer of a state label.
    io.apply_events(args.repo, number, station_event=d.station_event, vocabulary_path=args.vocabulary)
    if station == "archive":
        comment(args.repo, number,
                f"{STATION_MARKER['archive']}\n"
                f"Archiving this change: {where} started, approved by {approver}.\n\n"
                f"It archives the change on its branch and opens the archive pull request closing "
                f"this issue, unlabelled -- the session places no label on its own work. A workflow "
                f"carries `{vocab['run_label']}` forward as `{vocab['approve_label']}` onto that pull "
                f"request, and a person merges it. Nothing merges without a person.")
        print(f"fired the archive station for #{number}" + (f": {url}" if url else ""))
        return 0
    carries = vocab["run_label"] in {l.get("name") for l in issue.get("labels") or [] if l.get("name")}
    what = (f"a workflow reads `{vocab['run_label']}` again and carries it forward as "
            f"`{vocab['approve_label']}`, so the review's findings are fixed without a reply in each thread"
            if carries else
            "a person places a label to start the fixing loop")
    ends = (f"a workflow carries `{vocab['run_label']}` forward again to archive, once the "
            f"pull request merges — if this change is bound to an openspec change"
            if carries else
            "archiving is a person's own step too, same as merging")
    comment(args.repo, number,
            f"{MARKER}\n"
            f"Implementing this issue: {where} started, approved by {approver}.\n\n"
            f"It proposes a change, implements it on its own branch and opens a pull request "
            f"referencing this issue, unlabelled — the session places no label on its own work. "
            f"When the pull request opens, {what}. "
            f"Nothing merges without a person, and {ends}.")
    print(f"fired for #{number}" + (f": {url}" if url else ""))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except RuntimeError as exc:
        print(f"::error::{exc}")
        sys.exit(1)
