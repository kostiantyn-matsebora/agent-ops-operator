#!/usr/bin/env python3
"""The conveyor's state machine: every decision and every transition, no I/O.

THE LINE IS TWO MACHINES AND A SET OF DECISIONS, AND THIS FILE IS ALL OF IT.

  the STATION machine   on the tracking ISSUE, label `station:*`: where the
                        change is on its way from proposal to archive
  the LOOP machine      on a pull request, label `loop:*`: what the fixing
                        loop is doing on it
  the DECISIONS         fire, carry, gate, guard, ending, check: each reads
                        FACTS, applies one rule, and answers with a Decision
                        naming the action and the EVENTS the machines take

The workflows and the programs they call are ADAPTERS. They gather facts
(labels, who placed them, what the change's tasks say, how many rounds ran),
hand them to a function below, and do what it answers. They decide nothing.
A label is written only by `conveyor-state.py`, from an EVENT, through the
transition tables here -- so a transition this file does not define cannot be
written, and a disagreement between two programs about a rule cannot exist,
because there is one copy.

WHY IT IS ONE FILE. The line was six programs and three workflow scripts, each
holding its own copy of a rule, and the copies drifted. Each measured failure
below was one copy patched while its twin was not:

  #248  a timed-out round counted for nothing, so `max_rounds` never applied,
        while `docs-task` refused on "a round is running", which turned
        `ci-green` red, which started the next round: hours of rounds at
        "0 of 5", ended only by removing the grant by hand.
  #254  the carry accepted `conveyor:archive` as the archive pull request's
        grant, while the gate re-checked `conveyor:run` alone: every round
        refused.
  #255  the archive carry fired on a PROPOSAL merge, since nothing asked
        whether the change was finished, and `conveyor:run` fired the implement
        station on an issue whose change had already merged.
  also  nothing enforced the cap BEFORE a round started: after "round cap
        reached" the pushed commit's review completion started round cap+1.

`conveyor.test.py` enumerates every state and event of both machines and every
combination of facts each decision reads.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import re
import sys
from dataclasses import dataclass, fields

MAY_PUSH = {"admin", "maintain", "write"}
WORKFLOW_BOT = "github-actions[bot]"
SKIP = None  # a table entry meaning: the event does not move this state, and that is deliberate


# ==== the two machines ==========================================================================
#
# A TABLE, NOT LOGIC. Every (state, event) pair is listed, so the totality test
# can say "no pair is undefined" and a reader can see what an event does to
# every state. `SKIP` is an answer, not a gap.

STATION_STATES = ("none", "implement", "fix", "merge", "archive", "stalled", "done")
STATION_EVENTS = (
    "fire:implement",            # the implement station's session starts
    "fire:archive",              # the archive station's session starts
    "round:start",               # a fixing round starts on the change's pull request
    "pr:green",                  # the pull request is green with no review thread open
    "merge:proposal_or_apply",   # a pull request merged and the change is NOT finished
    "merge:finished_carried",    # the change's pull request merged, finished, and the grant was carried
    "merge:finished_stalled",    # ...finished, and there was no grant to carry
    "merge:plain",               # the plain lane's pull request merged: its line ends here
    "merge:archive_pr",          # the archive pull request merged: the line is over
)


def _station_table() -> dict:
    t = {}
    for s in STATION_STATES:
        for e in STATION_EVENTS:
            t[(s, e)] = SKIP
    live = [s for s in STATION_STATES if s != "done"]          # `done` is terminal: every event skips
    for s in live:
        t[(s, "fire:archive")] = "archive"
        t[(s, "round:start")] = "fix"
        t[(s, "pr:green")] = "merge"
        t[(s, "merge:proposal_or_apply")] = "implement"        # the next station is implement
        t[(s, "merge:finished_carried")] = "archive"
        t[(s, "merge:finished_stalled")] = "stalled"
        t[(s, "merge:plain")] = "done"
        t[(s, "merge:archive_pr")] = "done"
        # An implement session never REGRESSES a change that reached the archive
        # station: the fire decision refuses it, and the label must agree.
        t[(s, "fire:implement")] = SKIP if s == "archive" else "implement"
    return t


STATION_TABLE = _station_table()

LOOP_STATES = ("none", "running", "stalled", "capped", "mergeable")
LOOP_EVENTS = (
    "round:start",       # a round begins
    "end:continue",      # a round landed a commit: CI and the review run again
    "end:mergeable",     # a round found nothing left and the head is green
    "end:stalled",       # a round ended for a person: disputes, no report, failure, timeout, a thread
    "end:capped",        # the round counted and reached the ceiling
    "ci:green_clean",    # CI succeeded and no review thread is open
    "thread:opened",     # a review-authored thread is unresolved
    "recover:stalled",   # a superseded round left `running` behind and a thread is open
    "recover:capped",    # ...and the rounds used exceed the ceiling
)


def _loop_table() -> dict:
    t = {(s, e): SKIP for s in LOOP_STATES for e in LOOP_EVENTS}
    for s in LOOP_STATES:
        t[(s, "round:start")] = "running"
        t[(s, "end:continue")] = "running"
        t[(s, "end:mergeable")] = "mergeable"
        t[(s, "end:stalled")] = "stalled"
        t[(s, "end:capped")] = "capped"
    for s in ("none", "stalled", "capped", "mergeable"):
        # A ROUND OWNS THE LABEL WHILE IT RUNS. `running` skips these two: CI
        # succeeding on an earlier head, or a thread opening, must not overwrite
        # a label whose ending is still to come (measured on #225: the label
        # stuck at `running` on a green PR; and on #226: `mergeable` for an hour
        # after new findings -- each the other side of this one rule).
        t[(s, "ci:green_clean")] = "mergeable"
    t[("mergeable", "thread:opened")] = "stalled"
    t[("running", "recover:stalled")] = "stalled"
    t[("running", "recover:capped")] = "capped"
    return t


LOOP_TABLE = _loop_table()


def station_next(state: str, event: str) -> str | None:
    """The station after `event`, or None where the event leaves it as it is."""
    if (state, event) not in STATION_TABLE:
        raise ValueError(f"station machine: unknown ({state!r}, {event!r})")
    return STATION_TABLE[(state, event)]


def loop_next(state: str, event: str) -> str | None:
    """The loop state after `event`, or None where the event leaves it as it is."""
    if (state, event) not in LOOP_TABLE:
        raise ValueError(f"loop machine: unknown ({state!r}, {event!r})")
    return LOOP_TABLE[(state, event)]


# ==== facts and decisions ========================================================================

@dataclass(frozen=True)
class Placer:
    """Who placed a label, as the platform reports it."""
    login: str
    permission: str = "none"   # admin | maintain | write | triage | read | none
    bot: bool = False           # the workflow's own bot: a label CARRIED, never a person's own

    @property
    def may_push(self) -> bool:
        return self.permission in MAY_PUSH


@dataclass(frozen=True)
class Line:
    """The tracking issue and the change bound to it, as they stand now."""
    issue_labels: frozenset = frozenset()
    lane: str = "plain"                 # opsx | plain
    change_finished: bool = False       # every task ticked and the guard's shape satisfied, on the default branch
    fired: frozenset = frozenset()      # stations carrying a fire record: {"implement", "archive"}
    open_pr_from_change: bool = False   # a pull request is open from change/<name>: a session is at work


@dataclass(frozen=True)
class PullRequest:
    """A pull request of the line, as far as the line cares."""
    same_repo_change_branch: bool = True
    state: str = "OPEN"                 # OPEN | MERGED | CLOSED
    labels: frozenset = frozenset()
    refs: int | None = None             # `Refs #<n>`
    closes: int | None = None           # `Closes #<n>`
    review_completed: bool = False      # the review ran to completion on the head
    thread_open: bool = False           # a review-authored thread is unresolved
    ci_conclusion: str = ""             # success | failure | ...
    rounds_used: int = 0                # round markers since the fix label was placed
    grants: int = 0                     # grant markers since then: one per consumed keep-going
    max_rounds: int = 5


@dataclass(frozen=True)
class Decision:
    """What the adapter does. Every field is an instruction or a reason."""
    action: str
    reason: str = ""
    station: str = ""            # the station a fire or carry concerns
    station_event: str = ""      # the event for the station machine, applied by conveyor-state.py
    loop_event: str = ""         # the event for the loop machine
    remove_label: str = ""       # a label the adapter strips, on a refusal
    grant: str = ""              # the grant this decision relied on
    dispatch_round: bool = False
    counted: bool = False        # the round spent the budget


def cap_for(max_rounds: int, grants: int) -> int:
    """The effective ceiling: each consumed grant adds a full set."""
    return max_rounds * (1 + grants)


def count_marked(comments, marker: str, since: str = "") -> int:
    """How many comments carry `marker`, counting only those created at or after `since`.
    THE ONE COUNTER: the gate, the landing and the recovery all count rounds and grants with it."""
    return sum(1 for c in comments
               if marker in (c.get("body") or "") and (not since or (c.get("created_at") or "") >= since))


def is_finished(tasks_text: str) -> bool:
    """A change is finished when it has tasks and every one is ticked.

    docs-task-guard.py judges the SHAPE (the three trailing sections, in order);
    this is the plain question the archive carry needs: is anything still open?
    An absent or empty file is not finished."""
    boxes = re.findall(r"^\s*- \[([ xX])\]", tasks_text or "", re.M)
    return bool(boxes) and all(b in "xX" for b in boxes)


# ---- the one grant rule ----------------------------------------------------------------------

def standing_grant(vocab: dict, issue_labels, station: str, pr_closes: bool = False) -> str | None:
    """The label on the ISSUE that authorises `station`, or None.

    `conveyor:run` stands for every station. `conveyor:archive` stands for the
    archive station, and for the fix station OF THE ARCHIVE PULL REQUEST (the
    one that says `Closes #<n>`), because driving that pull request to
    mergeable is the archive station's own promise. It stands for nothing
    else: a pull request that merely proposes or applies a change is not the
    archive's to fix.

    EVERY re-check of a carried grant calls this: the fire, the carries and the
    gate. That is the whole of the #254 fix.
    """
    labels = set(issue_labels)
    if vocab["run_label"] in labels:
        return vocab["run_label"]
    if vocab["archive_label"] in labels and (station == "archive" or (station == "fix" and pr_closes)):
        return vocab["archive_label"]
    return None


def _carried_by_a_person_or_a_bot(placer: Placer | None) -> bool:
    return placer is not None and placer.may_push


# ---- the fire: a label on the issue ----------------------------------------------------------

def fire(vocab: dict, label: str, line: Line, placer: Placer) -> Decision:
    """A label landed on the issue. Which station starts, if any.

    Actions: `ignore` (not a label this line acts on), `refuse` (strip the label
    and say why), `skip` (a session is already at this station), `fire`.

    THE STATION FOLLOWS THE CHANGE. `conveyor:run` used to fire the implement
    station whatever the line was at. A finished change on the opsx lane is at
    the archive station and anything else is at implement; `conveyor:archive`
    names its station outright and is refused where the change is not finished
    or the lane has no archive.

    A CARRIED PLACEMENT NEEDS `conveyor:run` STANDING, since only the standing
    instruction can be carried, and it never fires twice.

    A PERSON'S PLACEMENT IS A FRESH INSTRUCTION. The fire record used to be
    permanent, so a session that died left an issue nobody could restart. A
    person re-placing a label fires again when no pull request is open from the
    change's branch (an open one means a session is still at work).
    """
    known = {vocab["implement_label"], vocab["run_label"], vocab["archive_label"]}
    if label not in known:
        return Decision("ignore", f"`{label}` is not a label this line acts on")

    strip = label
    if placer.bot:
        if vocab["run_label"] not in line.issue_labels:
            return Decision("refuse", f"`{label}` was carried by the workflow, but no `{vocab['run_label']}` "
                            "stands on the issue to carry", remove_label=strip)
    elif not placer.may_push:
        return Decision("refuse", f"`{label}` starts a session that writes to this repository, which needs "
                        f"write access, and @{placer.login} has `{placer.permission}`", remove_label=strip)

    if label == vocab["archive_label"] or (label == vocab["run_label"] and line.lane == "opsx" and line.change_finished):
        station = "archive"
    else:
        station = "implement"

    if station == "archive":
        keep = "" if label != vocab["archive_label"] else strip
        if line.lane != "opsx":
            return Decision("refuse", "the plain lane has no archive station; its line ended at the merge",
                            remove_label=keep)
        if not line.change_finished:
            return Decision("refuse", "the change is not finished (tasks are still open), so its next station "
                            "is implement, not archive", remove_label=keep)

    if station in line.fired:
        if placer.bot:
            return Decision("skip", f"the {station} station already carries a fire record and a carry never "
                            "fires twice", station=station)
        if line.open_pr_from_change:
            return Decision("skip", f"the {station} station already fired and a pull request is open from the "
                            "change's branch: a session is still at work", station=station)
        return Decision("fire", f"the {station} station fired before but nothing is open from the change's "
                        f"branch, so @{placer.login}'s placement is a fresh instruction",
                        station=station, station_event=f"fire:{station}")
    return Decision("fire", f"the {station} station starts", station=station, station_event=f"fire:{station}")


# ---- the carries -----------------------------------------------------------------------------

def carry_fix(vocab: dict, pr: PullRequest, line: Line, placer: Placer | None) -> Decision:
    """A pull request's CI completed. Carry the standing grant onto it as `conveyor:fix`.

    Actions: `nothing` (with the reason) or `carry`.

    A ROUND IS DISPATCHED ONLY WHEN THE REVIEW HAS ALREADY COMPLETED for the
    head. Otherwise the review's own completion starts it, and dispatching here
    too made two rounds of one push, both spending the budget.
    """
    if not pr.same_repo_change_branch:
        return Decision("nothing", "not a same-repository change/* pull request")
    issue = pr.refs or pr.closes
    if issue is None:
        return Decision("nothing", "the pull request names no issue (`Refs #<n>` or `Closes #<n>`)")
    grant = standing_grant(vocab, line.issue_labels, "fix", pr_closes=pr.closes is not None)
    if grant is None:
        return Decision("nothing", f"#{issue} carries no grant for this pull request's fix station")
    if not _carried_by_a_person_or_a_bot(placer):
        return Decision("nothing", f"`{grant}` on #{issue} was placed by someone who cannot push here now")
    green = pr.ci_conclusion == "success" and not pr.thread_open
    return Decision("carry", f"carried `{grant}` from #{issue}", station="fix", grant=grant,
                    dispatch_round=pr.review_completed,
                    loop_event="ci:green_clean" if green else "",
                    station_event="pr:green" if green else "")


def carry_archive(vocab: dict, pr: PullRequest, line: Line, placer: Placer | None) -> Decision:
    """A pull request from change/* merged. Carry the instruction onward.

    Actions: `nothing`, `done` (the line is over), `stalled` (the merge left
    nothing to carry), `carry` (label the issue and fire the archive station).

    ONLY A FINISHED CHANGE REACHES THE ARCHIVE STATION. A proposal or an apply
    merge comes from the same branch under the same `Refs #<n>`, and the carry
    used to read every one of them as the implementation merge (#255). An
    unfinished merge moves the line to implement instead.

    ONLY `conveyor:run` IS CARRIED. `conveyor:archive` standing alone was placed
    by a person for this station directly and has already fired.
    """
    if not pr.same_repo_change_branch:
        return Decision("nothing", "not a same-repository change/* pull request")
    if pr.closes is not None and pr.refs is None:
        return Decision("done", f"the pull request closes #{pr.closes}: the line ended at this merge",
                        station_event="merge:archive_pr")
    if pr.refs is None:
        return Decision("nothing", "the pull request's body carries no `Refs #<n>`")
    if line.lane != "opsx":
        return Decision("done", f"#{pr.refs} is on the plain lane, which has no archive station",
                        station_event="merge:plain")
    if not line.change_finished:
        return Decision("nothing", f"#{pr.refs}'s change is not finished, so this merge is a proposal or an "
                        "apply and its next station is implement", station_event="merge:proposal_or_apply")
    if vocab["run_label"] not in line.issue_labels:
        if vocab["archive_label"] in line.issue_labels:
            return Decision("nothing", f"#{pr.refs} carries `{vocab['archive_label']}` placed directly; the "
                            "archive station fired from it")
        return Decision("stalled", f"#{pr.refs} carries no `{vocab['run_label']}`, so the merge station has "
                        "nothing to carry onward", station_event="merge:finished_stalled")
    if not _carried_by_a_person_or_a_bot(placer):
        return Decision("nothing", f"`{vocab['run_label']}` on #{pr.refs} was placed by someone who cannot "
                        "push here now")
    return Decision("carry", f"carried `{vocab['run_label']}` from #{pr.refs}", station="archive",
                    grant=vocab["run_label"], station_event="merge:finished_carried")


# ---- the gate: may a round start -------------------------------------------------------------

@dataclass(frozen=True)
class Trigger:
    event: str                      # labeled | comment | review_completed | ci_failed | dispatch
    sender: Placer = Placer(WORKFLOW_BOT, bot=True)
    label: str = ""                 # for `labeled`
    comment_is_dispatch: bool = False


def gate(vocab: dict, trigger: Trigger, pr: PullRequest, line: Line, grant_placer: Placer | None) -> Decision:
    """Something asked for a round. Does one start.

    Actions: `round` (over everything open), `threads` (the accepted findings
    alone, from a person's dispatch), `none`, `refuse` (with `remove_label`
    where a label was the ask).

    A BOT'S START IS RE-CHECKED AGAINST THE SAME RULE THE CARRY USED, on every
    path that a bot can start a round by. The gate held three copies of this
    check, each grepping for `conveyor:run` alone, and the review-completion
    path held none (#254).

    THE BOUND IS ENFORCED HERE, BEFORE THE ROUND. It used to be counted after
    the fact, so the commit a capped round pushed started the next round through
    its review's completion. A round starts only while rounds used are below
    the ceiling, or while `conveyor:keep-going` stands to extend it.
    """
    fix, keep = vocab["approve_label"], vocab["keep_going_label"]
    if not pr.same_repo_change_branch:
        return Decision("refuse", "the pull request comes from a fork, and a dispatch only lands on a branch "
                        "of this repository")

    if trigger.event == "comment":
        if not trigger.comment_is_dispatch:
            return Decision("none", "not a dispatch")
        if not trigger.sender.may_push:
            return Decision("refuse", f"write access is required to dispatch a fix, and "
                            f"@{trigger.sender.login} has `{trigger.sender.permission}`")
        return Decision("threads", f"dispatch by @{trigger.sender.login}")

    if trigger.sender.bot:
        # A carried start: every path a bot can start a round by is re-checked.
        issue = pr.refs or pr.closes
        grant = standing_grant(vocab, line.issue_labels, "fix", pr_closes=pr.closes is not None) if issue else None
        if trigger.event in ("labeled", "dispatch") and (grant is None or not _carried_by_a_person_or_a_bot(grant_placer)):
            return Decision("refuse", "the workflow started this round carrying a grant, and re-checking the "
                            "issue found none standing for this pull request's fix station",
                            remove_label=trigger.label if trigger.event == "labeled" else "")
    elif trigger.event == "labeled" and not trigger.sender.may_push:
        return Decision("refuse", f"write access is required to place `{trigger.label}`, and "
                        f"@{trigger.sender.login} cannot push here", remove_label=trigger.label)

    if pr.state != "OPEN" or fix not in pr.labels:
        return Decision("none", f"the pull request is {pr.state.lower()} or does not carry `{fix}`")

    cap = cap_for(pr.max_rounds, pr.grants)
    if pr.rounds_used >= cap and keep not in pr.labels:
        return Decision("none", f"the loop reached its bound: {pr.rounds_used} of {cap} rounds used. "
                        f"`{keep}` grants another {pr.max_rounds}", loop_event="end:capped")
    return Decision("round", "a round starts over everything open", loop_event="round:start",
                    station_event="round:start")


# ---- the guard -------------------------------------------------------------------------------

def guard(vocab: dict, purpose: str, pr: PullRequest, running_rounds: int, unanswered_disputes: int) -> Decision:
    """`ci`: the documentation check's question. `archive`: the archive command's.

    Actions: `allow`, `refuse`.

    A CHECK NEVER CARRIES THE LOOP'S OWN STATE. Whether a round is running is
    moved by the loop, and a check reporting it red is a red the loop made,
    which starts the next round (#248). The check asks one thing: is a dispute
    the loop posted waiting for a person. The archive command asks both, since
    it acts on the branch a round may push to.
    """
    if purpose not in ("ci", "archive"):
        raise ValueError(f"guard purpose {purpose!r}")
    if pr.state != "OPEN":
        return Decision("allow", f"the pull request is {pr.state.lower()}; no loop can run on it")
    if vocab["approve_label"] not in pr.labels:
        return Decision("allow", f"the pull request does not carry `{vocab['approve_label']}`; nothing to wait for")
    reasons = []
    if purpose == "archive" and running_rounds:
        reasons.append("a fixing round is still running")
    if unanswered_disputes:
        reasons.append("the fixing step disputed a finding and no person has answered")
    if reasons:
        return Decision("refuse", "; ".join(reasons))
    return Decision("allow", "no dispute is waiting" + ("" if purpose == "ci" else " and no round is running"))


# ---- the endings -----------------------------------------------------------------------------

ENDINGS = ("landed", "clean", "timed out", "failed", "no report", "disputed", "stale patch", "waiting")
NO_MODEL_RAN = ("clean", "failed")   # the two endings that spent no model: nothing was accepted / the job never started


def ending(kind: str, number: int, cap: int, thread_open: bool = False, waiting_on_person: bool = False) -> Decision:
    """A round ended. The loop event, and whether the round counted.

    `number` is this round's number and `cap` the current ceiling.

    EVERY ROUND THAT RAN A MODEL COUNTS, whatever it ended as. A landed round
    spent the budget, and so did a round that ran to its time limit (#248:
    three thirty-minute rounds at "0 of 5"), one that disputed everything, one
    that wrote no report and one whose patch went stale. Only a round in which
    no model ran spends nothing: a clean one (nothing was accepted, so the
    fixing job was skipped) and a fixing job that failed before starting. A
    counted round that reaches the ceiling ends the loop.
    """
    if kind not in ENDINGS:
        raise ValueError(f"unknown ending {kind!r}")
    counted = kind not in NO_MODEL_RAN
    if counted and number >= cap:
        return Decision("capped", f"{cap} rounds have run", loop_event="end:capped", counted=True)
    if kind == "landed":
        return Decision("continue", "the push starts CI and the review", loop_event="end:continue", counted=True)
    if kind == "clean":
        if thread_open or waiting_on_person:
            return Decision("stalled", "nothing to fix, but a person's answer is waited on", loop_event="end:stalled")
        return Decision("mergeable", "nothing left and the head is green", loop_event="end:mergeable")
    return Decision("stalled", f"the round ended: {kind}", loop_event="end:stalled", counted=counted)


# ---- failed checks ---------------------------------------------------------------------------

def check_is_work(job: str, conclusion: str, failed_steps, guard_step: str, required) -> Decision:
    """A check run on the head. Is it something a fixer may act on.

    Actions: `skip` (with the reason), `waiting`, `work`.

    A RED MADE ONLY OF THE LOOP'S OWN GUARD IS NOT WORK. The documentation check
    failing on an unanswered dispute waits for the person it names, and no
    fixer can answer for them; it is `waiting`, so the round's ending says what
    it waits on rather than calling the head clean.
    """
    name = job.split(" (", 1)[0].strip()
    if name == "ci-green":
        return Decision("skip", "the aggregate")
    if name not in required:
        return Decision("skip", "not a required check")
    if conclusion != "failure":
        return Decision("skip", f"concluded {conclusion or 'nothing'}")
    steps = [s for s in failed_steps if s]
    if guard_step and steps and all(s == guard_step for s in steps):
        return Decision("waiting", "failed only on the loop's own guard: a dispute waits for a person")
    return Decision("work", "a failed required check")


# ---- recovery and refresh: correcting a label a round left behind ----------------------------

def recover(current: str, rounds_used: int, cap: int, thread_open: bool) -> str:
    """The loop event for a `running` label whose round was never reported, or ''."""
    if current != "running":
        return ""
    if rounds_used > cap:
        return "recover:capped"
    if thread_open:
        return "recover:stalled"
    return ""


def refresh(current: str, thread_open: bool) -> str:
    """The loop event for a review that completed without starting a round, or ''."""
    return "thread:opened" if thread_open and current == "mergeable" else ""


# ==== JSON command line, for the adapters that are shell =========================================

def _build(cls, data: dict):
    names = {f.name for f in fields(cls)}
    out = {}
    for k, v in (data or {}).items():
        if k not in names:
            raise ValueError(f"{cls.__name__} has no fact {k!r}")
        out[k] = frozenset(v) if k in ("issue_labels", "labels", "fired") else v
    return cls(**out)


def _placer(d):
    return None if d is None else _build(Placer, d)


def _decide(vocab: dict, name: str, facts: dict) -> dict:
    from dataclasses import asdict
    if name == "fire":
        d = fire(vocab, facts["label"], _build(Line, facts.get("line")), _placer(facts["placer"]))
    elif name == "carry_fix":
        d = carry_fix(vocab, _build(PullRequest, facts.get("pr")), _build(Line, facts.get("line")), _placer(facts.get("placer")))
    elif name == "carry_archive":
        d = carry_archive(vocab, _build(PullRequest, facts.get("pr")), _build(Line, facts.get("line")), _placer(facts.get("placer")))
    elif name == "gate":
        t = dict(facts["trigger"]); t["sender"] = _placer(t.get("sender", {"login": WORKFLOW_BOT, "bot": True}))
        d = gate(vocab, Trigger(**t), _build(PullRequest, facts.get("pr")), _build(Line, facts.get("line")), _placer(facts.get("grant_placer")))
    elif name == "guard":
        d = guard(vocab, facts["purpose"], _build(PullRequest, facts.get("pr")), facts.get("running_rounds", 0), facts.get("unanswered_disputes", 0))
    elif name == "ending":
        d = ending(facts["kind"], facts["number"], facts["cap"], facts.get("thread_open", False), facts.get("waiting_on_person", False))
    else:
        raise ValueError(f"no decision {name!r}")
    return asdict(d)


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description="the conveyor's decisions, for shell callers")
    ap.add_argument("--vocabulary", type=pathlib.Path,
                    default=pathlib.Path(__file__).resolve().parents[1] / "review-triage.json")
    sub = ap.add_subparsers(dest="cmd", required=True)
    d = sub.add_parser("decide", help="read facts as JSON on stdin, print the Decision as JSON")
    d.add_argument("decision", choices=["fire", "carry_fix", "carry_archive", "gate", "guard", "ending"])
    n = sub.add_parser("next", help="print the state after an event, or the state unchanged when the event skips")
    n.add_argument("machine", choices=["station", "loop"])
    n.add_argument("state")
    n.add_argument("event")
    args = ap.parse_args(argv)
    if args.cmd == "next":
        fn = station_next if args.machine == "station" else loop_next
        nxt = fn(args.state, args.event)
        print(args.state if nxt is None else nxt)
        return 0
    vocab = json.loads(args.vocabulary.read_text())
    print(json.dumps(_decide(vocab, args.decision, json.load(sys.stdin))))
    return 0


if __name__ == "__main__":
    sys.exit(main())
