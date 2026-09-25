"""The conveyor's facts, gathered once.

`conveyor.py` decides and does no I/O. Every adapter needs the same handful of
facts to hand it -- who placed a label and whether they may push, which lane an
issue is on, whether its change is finished, which stations already fired, whether
a session's pull request is open -- and each program used to fetch its own, with
its own slightly different reading. They are gathered here, the same way for all
of them, and returned as the machine's own types.

EVERY READ FAILS CLOSED WHERE A WRONG ANSWER STARTS WORK. An unreadable comment
list means "already fired", never "not yet", because a second session on one
branch is the case the fire record exists to prevent.
"""
from __future__ import annotations

import json
import os
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
FIRE_MARKERS = {"implement": "<!-- remote-implement:fired -->", "archive": "<!-- remote-implement:fired:archive -->"}


def gh(*args: str, check: bool = True) -> str:
    out = subprocess.run(["gh", *args], capture_output=True, text=True)
    if check and out.returncode != 0:
        raise RuntimeError(f"gh {' '.join(args)}: {out.stderr.strip()}")
    return out.stdout.strip()


def vocabulary(path: pathlib.Path = DEFAULT_VOCABULARY) -> dict:
    return json.loads(path.read_text())


def write_output(name: str, value: str) -> None:
    """A step output for the workflow, when there is one. Text goes to stdout and
    stderr for people; this is what a later step's `if:` reads."""
    target = os.environ.get("GITHUB_OUTPUT")
    if target:
        with open(target, "a") as f:
            f.write(f"{name}={value}\n")


def parse_paginated(raw: str) -> list:
    """`gh api --paginate` without `--jq` concatenates each page's JSON array back
    to back rather than merging them, so a bare `json.loads` raises on any target
    whose timeline spans more than one page."""
    try:
        return json.loads(raw or "[]")
    except json.JSONDecodeError:
        events: list = []
        for chunk in raw.replace("][", "]\n[").splitlines():
            if chunk.strip():
                events.extend(json.loads(chunk))
        return events


def issue_labels(repo: str, issue: int) -> frozenset:
    raw = gh("issue", "view", str(issue), "--repo", repo, "--json", "labels")
    try:
        doc = json.loads(raw or "{}")
    except json.JSONDecodeError:
        return frozenset()
    return frozenset(l.get("name") for l in doc.get("labels") or [] if l.get("name"))


def label_placement(repo: str, target: int, label: str) -> tuple[str, str] | None:
    """(login, iso timestamp) of the LATEST placement of `label` on the target's
    timeline, or None when the timeline shows nobody placing it. The latest
    counts, so removing and re-adding a label starts a fresh count."""
    try:
        events = parse_paginated(gh("api", f"repos/{repo}/issues/{target}/timeline", "--paginate"))
    except (RuntimeError, json.JSONDecodeError):
        return None
    placements = [e for e in events if e.get("event") == "labeled" and (e.get("label") or {}).get("name") == label]
    if not placements:
        return None
    last = placements[-1]
    login, when = (last.get("actor") or {}).get("login"), last.get("created_at")
    return (login, when) if login and when else None


def permission(repo: str, login: str) -> str:
    """What the platform says this person may do here. UNREADABLE IS `none`: this
    answers a security gate, and a transient error must decline the carry rather
    than crash it."""
    try:
        return gh("api", f"repos/{repo}/collaborators/{login}/permission", "--jq", ".permission")
    except RuntimeError:
        return "none"


def placer(repo: str, login: str) -> conveyor.Placer:
    """A login as the machine's `Placer`: the workflow's own bot is a CARRIED
    placement and never a person's own, and everyone else is asked of the platform."""
    if login == conveyor.WORKFLOW_BOT:
        return conveyor.Placer(login, "none", bot=True)
    return conveyor.Placer(login, permission(repo, login))


def grant_placer(repo: str, issue: int, grant_label: str) -> conveyor.Placer | None:
    """The PERSON behind a standing grant: who placed `grant_label` on the issue,
    as a Placer. None when the timeline shows nobody, which the machine reads as
    "cannot push", so a grant nobody demonstrably placed carries nothing."""
    found = label_placement(repo, issue, grant_label)
    return placer(repo, found[0]) if found else None


def bound_change(issue: int) -> pathlib.Path | None:
    """The change directory whose `.github-issue` names this issue, or None."""
    for sidecar in sorted(pathlib.Path(".").glob("openspec/changes/*/.github-issue")):
        try:
            if int(sidecar.read_text().strip()) == issue:
                return sidecar.parent
        except (OSError, ValueError):
            continue
    return None


def lane(repo: str, issue: int, labels: frozenset) -> str:
    """`opsx` when a change is bound to the issue or it carries an `opsx:` phase
    label, else `plain`. Both are FACTS read, never a judgement of how the issue
    is worded."""
    if bound_change(issue) is not None or any(l.startswith("opsx:") for l in labels):
        return "opsx"
    return "plain"


def change_finished(issue: int) -> bool:
    """The bound change's tasks are all ticked, on the checkout's branch. No bound
    change, or an unreadable file, is not finished."""
    change = bound_change(issue)
    if change is None:
        return False
    try:
        return conveyor.is_finished((change / "tasks.md").read_text())
    except OSError:
        return False


def fired_stations(repo: str, issue: int) -> frozenset | None:
    """The stations carrying a fire record, or None when the comments cannot be
    read -- which the caller must treat as EVERY station having fired."""
    try:
        raw = gh("api", f"repos/{repo}/issues/{issue}/comments", "--paginate", "--jq", ".[].body")
    except RuntimeError:
        return None
    return frozenset(st for st, marker in FIRE_MARKERS.items() if marker in raw)


def open_pr_from_change(repo: str, issue: int) -> bool | None:
    """A pull request is open from the change's branch: a session is at work.
    None when it cannot be read. The opsx lane's branch is `change/<name>`, and
    the plain lane's starts `change/<n>-`."""
    try:
        raw = gh("pr", "list", "--repo", repo, "--state", "open", "--limit", "200",
                 "--json", "headRefName", "--jq", ".[].headRefName")
    except RuntimeError:
        return None
    change = bound_change(issue)
    names = set(raw.split())
    if change is not None and f"change/{change.name}" in names:
        return True
    return any(n.startswith(f"change/{issue}-") for n in names)


def line(repo: str, issue: int, sessions: bool = True) -> conveyor.Line:
    """The tracking issue as the machine's `Line`. Unreadable fire records or open
    pull requests are read as the conservative answer, that a session is already
    at work, since the mistake to avoid is a second one. `sessions=False` skips
    those two reads for a caller (the carries) whose decision never looks at them."""
    labels = issue_labels(repo, issue)
    if not sessions:
        return conveyor.Line(issue_labels=labels, lane=lane(repo, issue, labels),
                             change_finished=change_finished(issue))
    fired = fired_stations(repo, issue)
    open_pr = open_pr_from_change(repo, issue)
    # UNREADABLE FAILS CLOSED, AS ONE FACT. If the fire records cannot be read we do not know
    # that a station has NOT fired, so every station is treated as fired AND a session as
    # open: a person's re-placement is only a fresh instruction where nothing is at work, and
    # a rate limit must never be the thing that starts a second session on one branch.
    blind = fired is None or open_pr is None
    return conveyor.Line(
        issue_labels=labels, lane=lane(repo, issue, labels), change_finished=change_finished(issue),
        fired=frozenset(FIRE_MARKERS) if fired is None else fired,
        open_pr_from_change=True if blind else open_pr)


def comment(repo: str, target: int, body: str) -> None:
    subprocess.run(["gh", "issue", "comment", str(target), "--repo", repo, "--body", body], check=False)


def already_marked(repo: str, target: int, marker: str) -> bool:
    """A marker comment stands on the target. A PULL REQUEST IS AN ISSUE TO THIS API."""
    try:
        return marker in gh("api", f"repos/{repo}/issues/{target}/comments", "--paginate", "--jq", ".[].body")
    except RuntimeError:
        return True   # unreadable: assume it stands, so nothing is posted twice


def apply_events(repo: str, target: int, station_event: str = "", loop_event: str = "",
                 vocabulary_path: pathlib.Path = DEFAULT_VOCABULARY) -> None:
    """Send the machine's events through `conveyor-state.py`, the one writer of a
    state label. It never fails the caller: a transition that could not be
    recorded is a notice, and the next transition derives from the live label."""
    script = pathlib.Path(__file__).with_name("conveyor-state.py")
    if not script.is_file():
        return
    for flag, event in (("--station-event", station_event), ("--loop-event", loop_event)):
        if event:
            subprocess.run([sys.executable, str(script), "--repo", repo, "--target", str(target), flag, event,
                            "--vocabulary", str(vocabulary_path)], check=False)
