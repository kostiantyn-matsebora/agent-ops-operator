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

MARKER = "<!-- remote-implement:fired -->"
BETA = "experimental-cc-routine-2026-04-01"
API_VERSION = "2023-06-01"
MAY_PUSH = {"admin", "maintain", "write"}


def gh(*args: str, check: bool = True) -> str:
    out = subprocess.run(["gh", *args], capture_output=True, text=True)
    if check and out.returncode != 0:
        raise RuntimeError(f"gh {' '.join(args)}: {out.stderr.strip()}")
    return out.stdout.strip()


def vocabulary(path: pathlib.Path) -> dict:
    return json.loads(path.read_text())


def permission(repo: str, login: str) -> str:
    """What the platform says this person may do here. Unreadable is `none`:
    a gate that fails OPEN would be no gate."""
    try:
        return gh("api", f"repos/{repo}/collaborators/{login}/permission", "--jq", ".permission")
    except RuntimeError:
        return "none"


def already_fired(repo: str, number: int) -> bool:
    """The marker is what makes the record ONCE. Re-labelling fires again by
    design -- the session finds the change already bound to the issue and
    continues it -- but a second comment on the same issue would read as two
    sessions racing."""
    try:
        raw = gh("api", f"repos/{repo}/issues/{number}/comments", "--paginate",
                 "--jq", ".[].body")
    except RuntimeError as exc:
        # UNREADABLE IS "ALREADY FIRED", not "fire again". Answering False on a
        # rate limit or a dropped connection starts a SECOND session on the same
        # issue — two sessions on one branch, which is the case the marker
        # exists to prevent. Refusing to fire is recoverable by re-labelling;
        # a duplicate run is not.
        print(f"::warning::could not read #{number}'s comments ({exc}); "
              "treating it as already fired rather than risking a second session")
        return True
    return MARKER in raw


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
    vocab = vocabulary(args.vocabulary)
    # EITHER LABEL FIRES THE SAME SESSION. `conveyor:implement` drives one
    # station by hand; `conveyor:run` is the standing instruction that carries
    # the change through every later station too — but starting the session is
    # the same act either way, and which one is on the issue is what the later
    # stations read to decide whether to carry themselves forward.
    want = {vocab["implement_label"], vocab["run_label"]}
    if label not in want:
        # ANOTHER LABEL. Not an error: this workflow sees every label event.
        print(f"label {label!r} is not in {sorted(want)!r}; nothing to do")
        return 0

    issue = event.get("issue") or {}
    number = issue.get("number")
    if not isinstance(number, int):
        print(f"::error::the event names no issue number ({number!r})")
        return 1
    if issue.get("pull_request"):
        # A PULL REQUEST IS AN ISSUE TO THIS API. The implement label belongs on
        # an issue; on a pull request it would fire a session to implement a
        # change that already has one.
        print(f"::notice::#{number} is a pull request, not an issue; nothing fires")
        return 0

    sender = (event.get("sender") or {}).get("login") or ""
    perm = permission(args.repo, sender)
    if perm not in MAY_PUSH:
        # REFUSED, VISIBLY, AND THE LABEL COMES OFF.
        subprocess.run(["gh", "issue", "edit", str(number), "--repo", args.repo,
                        "--remove-label", label], check=False)
        comment(args.repo, number,
                f"@{sender} placed `{label}`, which starts a session that writes to this "
                f"repository — that needs write access, and `{sender}` has `{perm}`. "
                f"The label was removed.")
        print(f"::error::{sender} has {perm}; the label was removed")
        return 1

    if already_fired(args.repo, number):
        print(f"::notice::#{number} already carries a fire record; not firing again")
        return 0

    # STRIPPED, BECAUSE A COPIED URL CARRIES A NEWLINE. `gh variable set` stores
    # whatever it is handed, a wrapped terminal display invites copying the line
    # break with it, and `urllib` then raises InvalidURL deep in http.client —
    # a stack trace on the runner and NOTHING on the issue, which is the one
    # place a person would look. Measured on the first live fire, 2026-09-09.
    fire_url = (args.fire_url or "").strip()
    token = os.environ.get("ROUTINE_FIRE_TOKEN", "").strip()
    # A BREAK INSIDE THE VALUE, not only at its ends. The stored variable held
    # `trig_01UBwPZ\nb9cN68hvcKxZTx2WH` — copied out of a wrapped display — and
    # `.strip()` cannot reach that. http.client refuses control characters four
    # frames down, so without this the run dies in a traceback and the issue,
    # the one place a person looks, says nothing at all.
    if fire_url and (any(c in fire_url for c in "\r\n\t ")
                     or not fire_url.lower().startswith(("http://", "https://"))):
        comment(args.repo, number,
                f"`{label}` was placed by @{sender}, but this repository's `ROUTINE_FIRE_URL` "
                "is not a URL. Nothing started; check the variable and place the label again.")
        print(f"::error::ROUTINE_FIRE_URL is not a URL: {fire_url[:60]!r}")
        return 1
    # THE TOKEN IS COPIED FROM THE SAME DIALOG AND BREAKS THE SAME WAY, and its
    # failure is worse to read: a header carrying a newline is refused by
    # http.client exactly as the url is, and one that merely lost characters
    # comes back 401 — indistinguishable from a revoked credential. NEVER print
    # or comment the value; the name is enough to fix it by.
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
    # THE SESSION OPENS ITS PULL REQUEST WITH NO LABEL. It acts as an
    # application with no write access, so a label it placed on its own work
    # would be removed by the gate that checks who labelled (#201). Where
    # `run_label` authorised this, a workflow reads the issue again once the
    # pull request opens and carries that instruction forward as
    # `approve_label` — recording whose it was — never the session itself.
    #
    # READ FROM THE ISSUE'S LABELS, NOT FROM WHICH LABEL FIRED THIS EVENT.
    # The later carry (`carry-grant.py`, at the `open` job) reads the issue's
    # CURRENT labels too, never which one triggered this session — so an
    # issue that already carries `run_label` when `implement_label` is the
    # one placed (the standing instruction pre-dating this particular
    # session) still gets it carried forward, and this comment must say so
    # rather than the "a person places a label" text that fits only when no
    # standing instruction exists at all.
    carries = vocab["run_label"] in {l.get("name") for l in issue.get("labels") or [] if l.get("name")}
    what = (f"a workflow reads `{label}` again and carries it forward as "
            f"`{vocab['approve_label']}`, so the review's findings are fixed without a reply in each thread"
            if carries else
            "a person places a label to start the fixing loop")
    # NOTHING MERGES WITHOUT A PERSON, EVER -- but archiving CAN be a
    # workflow's own unattended step once conveyor:run stands, on whichever
    # issue this session's change is bound to (see remote-session.md). This
    # job runs before the session has read the issue, so it does not yet
    # know which lane the session will choose -- the plain lane has no
    # archive station at all, and "always" here would misdescribe that case.
    ends = (f"a workflow carries `{label}` forward again to archive, once the "
            f"pull request merges — if this change is bound to an openspec change"
            if carries else
            "archiving is a person's own step too, same as merging")
    comment(args.repo, number,
            f"{MARKER}\n"
            f"Implementing this issue: {where} started, approved by @{sender}.\n\n"
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
