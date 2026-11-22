#!/usr/bin/env python3
"""The testing station's adapter: the facts in, the machine's answer acted on.

`conveyor.py` decides. This reads the facts, asks, and writes the labels and the one
report. It runs no model. The workflow `conveyor-testing.yml` runs the sessions between
these steps.

  begin         the pull request's issue, its standing grant, the station label, the round
                number and the station's ceiling. The implement -> testing hand-over happens here.
  judge         the tiers' coverage and test outcomes -> verdicts, the report on the issue,
                and the station's ending (passed, continue or capped), written as step outputs.
  record-round  one marker comment on the ISSUE per round in which a model ran: the budget's counter.
  check-patch   a session's patch against its role (tester: test files only, fixer: no
                tester-authored file). Exits 4 on a refusal.
  check-tree    the same refusals over the working tree, for an agent that edits in place. Exits 4.
  real-model    whether the change needs a real model (the path floor and the session's judgement),
                and whether the credential is set. Exits 6 when it is needed and absent.
  models        the tester's model against the author's. Exits 5 when they are not independent.

THE BUDGET IS COUNTED FROM THE ISSUE'S COMMENTS, as the fixing loop's is counted from the
pull request's: no store, so a re-run or a second runner reads the same number. The station
has its OWN markers (`testing.round_marker`, `testing.grant_marker`) so a fixing round never
spends it and it never spends the fixing loop's.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402
import conveyor_io as io  # noqa: E402


def _load_report():
    import importlib.util
    spec = importlib.util.spec_from_file_location("testing_report", pathlib.Path(__file__).with_name("testing-report.py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


report = _load_report()

REPORT_DIR = pathlib.Path("coverage-report")
TRAILER = "Conveyor-Tester: true"


def issue_of(body: str) -> int | None:
    import re
    m = re.search(r"\b(?:Refs|Closes)\s+#(\d+)", body or "")
    return int(m.group(1)) if m else None


def comments(repo: str, issue: int) -> list:
    raw = io.gh("api", f"repos/{repo}/issues/{issue}/comments", "--paginate")
    return io.parse_paginated(raw)


def budget(vocab: dict, thread: list) -> tuple[int, int]:
    """(rounds used, ceiling) for the station, from the issue's comments."""
    t = vocab["testing"]
    used = conveyor.count_marked(thread, t["round_marker"])
    grants = conveyor.count_marked(thread, t["grant_marker"])
    return used, conveyor.testing_cap(t["max_rounds"], grants)


def tester_authored(base: str) -> list:
    """Files the tester committed on this branch, found by the trailer its commits carry."""
    out = subprocess.run(["git", "log", f"origin/{base}..HEAD", "--format=", "--name-only", "--grep", TRAILER],
                         capture_output=True, text=True, check=False).stdout
    return sorted({f for f in out.splitlines() if f})


def patch_paths(patch: pathlib.Path) -> list:
    out = subprocess.run(["git", "apply", "--numstat", "-z", str(patch)], capture_output=True, text=True, check=True).stdout
    paths = []
    for rec in out.split("\0"):
        parts = rec.split("\t")
        if len(parts) == 3 and parts[2]:
            paths.append(parts[2])
    return paths


def cmd_begin(args, vocab) -> int:
    view = json.loads(io.gh("pr", "view", str(args.pr), "--repo", args.repo, "--json",
                            "isCrossRepository,headRefName,body,state,labels"))
    issue = issue_of(view.get("body") or "")
    same = (not view.get("isCrossRepository")) and (view.get("headRefName") or "").startswith("change/")
    if not (same and issue and view.get("state") == "OPEN"):
        print("not an open same-repository change/* pull request naming an issue: nothing to test")
        io.write_output("active", "false")
        return 0
    labels = io.issue_labels(args.repo, issue)
    grant = conveyor.standing_grant(vocab, labels, "testing")
    if grant is None:
        print(f"#{issue} carries no standing instruction: the testing station does not start unattended")
        io.write_output("active", "false")
        return 0
    state = vocab["station_labels"]
    if state["implement"] in labels:
        io.apply_events(args.repo, issue, station_event="testing:start", vocabulary_path=args.vocabulary)
        print(f"#{issue}: implement -> testing")
    elif state["testing"] not in labels and state["stalled"] not in labels:
        print(f"#{issue} is not at the testing station")
        io.write_output("active", "false")
        return 0
    resumed = False
    thread = comments(args.repo, issue)
    used, cap = budget(vocab, thread)
    keep = vocab["keep_going_label"]
    pr_labels = {l.get("name") for l in view.get("labels") or []}
    if used >= cap and keep in pr_labels:
        # CONSUMED ONLY WHEN IT TAKES EFFECT: the station was out of rounds and a person asked for more.
        io.comment(args.repo, issue, f"{vocab['testing']['grant_marker']}\n`{keep}` grants another "
                   f"{vocab['testing']['max_rounds']} testing rounds.")
        io.gh("issue", "edit", str(args.pr), "--repo", args.repo, "--remove-label", keep, check=False)
        if state["stalled"] in labels:
            io.apply_events(args.repo, issue, station_event="testing:resume", vocabulary_path=args.vocabulary)
        used, cap = budget(vocab, comments(args.repo, issue))
        resumed = True
    elif state["stalled"] in labels:
        print(f"#{issue} is stalled, and no `{keep}` asks for more testing rounds")
        io.write_output("active", "false")
        return 0
    io.write_output("active", "true")
    io.write_output("issue", str(issue))
    io.write_output("round", str(used + 1))
    io.write_output("cap", str(cap))
    io.write_output("exhausted", "true" if used >= cap else "false")
    io.write_output("resumed", "true" if resumed else "false")
    print(f"#{issue}: testing round {used + 1} of {cap}")
    return 0


def cmd_judge(args, vocab) -> int:
    covs = {}
    for kv in args.tier_file:
        tier, _, path = kv.partition("=")
        f = pathlib.Path(path)
        covs[tier] = json.loads(f.read_text()) if f.exists() and f.stat().st_size else None
    outcomes = dict(kv.split("=", 1) for kv in args.outcome)
    verdict = report.judge(vocab, covs, outcomes)
    work = [conveyor.WorkItem(**w) for w in verdict["work"]]
    d = conveyor.testing_ending(work, args.round, args.cap, changed=args.changed, model_ran=args.model_ran)
    state = {"passed": "passed", "capped": "capped"}.get(d.action, "running")
    body = report.render(vocab, verdict, args.round, args.cap, state)
    REPORT_DIR.mkdir(exist_ok=True)
    (REPORT_DIR / "verdict.json").write_text(json.dumps(verdict, indent=2, sort_keys=True))
    (REPORT_DIR / "report.md").write_text(body)
    print(report.post(vocab, args.issue, body, args.repo))
    if d.station_event:
        io.apply_events(args.repo, args.issue, station_event=d.station_event, vocabulary_path=args.vocabulary)
    if d.action == "capped":
        io.comment(args.repo, args.issue,
                   f"The testing station is stalled: {d.reason}. The report above names what is left. "
                   f"Place `{vocab['keep_going_label']}` on the pull request to grant another "
                   f"{vocab['testing']['max_rounds']} rounds, or take it from here.")
    if d.action == "passed" and args.pr:
        # THE FIXING LOOP STARTS ON THE HEAD THAT PASSED. A pass that pushes nothing fires no CI completion,
        # so nothing would carry the grant: ask for it explicitly.
        io.gh("workflow", "run", "remote-implement.yml", "--repo", args.repo, "-f", f"carry_fix_pr={args.pr}", check=False)
    io.write_output("ending", d.action)
    io.write_output("tester_work", "true" if any(w.session == "tester" for w in work) else "false")
    io.write_output("fixer_work", "true" if any(w.session == "fixer" for w in work) else "false")
    print(f"testing round {args.round}: {d.action}: {d.reason}")
    return 0


def cmd_record_round(args, vocab) -> int:
    io.comment(args.repo, args.issue, f"{vocab['testing']['round_marker']} {args.round} -->\n"
               f"Testing round {args.round} ran a session. This comment counts it against the station's budget.")
    return 0


def cmd_check_patch(args, vocab) -> int:
    paths = patch_paths(args.patch)
    if args.role == "tester":
        bad = conveyor.tester_patch_refusals(paths, vocab["testing"]["test_paths"])
        why = "the tester writes test files only"
    else:
        bad = conveyor.fixer_patch_refusals(paths, tester_authored(args.base))
        why = "a test the tester wrote is disputed, never edited"
    if bad:
        print(f"refused: {why}. Paths: {', '.join(bad)}")
        return 4
    print(f"the {args.role}'s patch touches {len(paths)} path(s), all allowed")
    return 0


def changed_paths(base: str) -> list:
    out = subprocess.run(["git", "diff", "--name-only", f"origin/{base}...HEAD"], capture_output=True, text=True, check=False).stdout
    return [p for p in out.splitlines() if p]


def cmd_check_tree(args, vocab) -> int:
    """After an agent has edited the working tree: which uncommitted paths its role refuses.
    A role is judged by what it TOUCHED, since the agent edits in place and writes no patch."""
    out = subprocess.run(["git", "status", "--porcelain", "-z", "-uall"], capture_output=True, text=True, check=True).stdout
    paths = sorted({rec[3:] for rec in out.split("\0") if len(rec) > 3})
    if args.role == "tester":
        bad = conveyor.tester_patch_refusals(paths, vocab["testing"]["test_paths"])
        why = "the tester writes test files only"
    else:
        bad = conveyor.fixer_patch_refusals(paths, tester_authored(args.base))
        why = "a test the tester wrote is disputed, never edited"
    if bad:
        print(f"refused: {why}. Paths: {', '.join(bad)}")
        return 4
    print(f"the {args.role} touched {len(paths)} path(s), all allowed")
    return 0


def cmd_real_model(args, vocab) -> int:
    """Whether this change needs a real model, from the diff and the session's judgement. Exits 6 on `stop`.
    The credential is only ever tested for PRESENCE: its value is never read into this program's output."""
    import os
    t = vocab["testing"]
    present = bool(os.environ.get(t["credential_env"], "").strip())
    d = conveyor.real_model_gate(changed_paths(args.base), t["agent_facing_paths"], args.session_requires, present)
    io.write_output("real_model", d.action)
    print(f"real model: {d.action}: {d.reason}")
    return 6 if d.action == "stop" else 0


def cmd_models(args, vocab) -> int:
    tester = args.tester or vocab["testing"]["tester_model"]
    if not conveyor.models_independent(tester, args.author):
        print(f"refused: the tester model ({tester or 'unset'}) is not independent of the author model "
              f"({args.author or 'unset'}). Set AUTHOR_MODEL, in the cloud environment, to the model the implement routine runs on.")
        return 5
    print(f"the tester ({tester}) is not the author ({args.author})")
    return 0


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--vocabulary", type=pathlib.Path, default=io.DEFAULT_VOCABULARY)
    sub = ap.add_subparsers(dest="cmd", required=True)

    b = sub.add_parser("begin")
    b.add_argument("--repo", required=True); b.add_argument("--pr", type=int, required=True)

    j = sub.add_parser("judge")
    j.add_argument("--repo", required=True); j.add_argument("--issue", type=int, required=True)
    j.add_argument("--round", type=int, required=True); j.add_argument("--cap", type=int, required=True)
    j.add_argument("--pr", type=int, default=0, help="the pull request: a passing station asks for the fixing loop's carry on it")
    j.add_argument("--tier-file", action="append", default=[], metavar="TIER=FILE")
    j.add_argument("--outcome", action="append", default=[], metavar="TIER=passed|failed|skipped")
    j.add_argument("--no-change", dest="changed", action="store_false")
    j.add_argument("--no-model", dest="model_ran", action="store_false")

    r = sub.add_parser("record-round")
    r.add_argument("--repo", required=True); r.add_argument("--issue", type=int, required=True)
    r.add_argument("--round", type=int, required=True)

    c = sub.add_parser("check-patch")
    c.add_argument("--role", choices=["tester", "fixer"], required=True)
    c.add_argument("--patch", type=pathlib.Path, required=True)
    c.add_argument("--base", default="master")

    t = sub.add_parser("check-tree")
    t.add_argument("--role", choices=["tester", "fixer"], required=True)
    t.add_argument("--base", default="master")

    rm = sub.add_parser("real-model")
    rm.add_argument("--base", default="master")
    rm.add_argument("--session-requires", action="store_true",
                    help="the session judged the model's behaviour relevant beyond the path floor")

    m = sub.add_parser("models")
    m.add_argument("--author", default="")
    m.add_argument("--tester", default="")

    args = ap.parse_args(argv)
    vocab = io.vocabulary(args.vocabulary)
    return {"begin": cmd_begin, "judge": cmd_judge, "record-round": cmd_record_round,
            "check-patch": cmd_check_patch, "check-tree": cmd_check_tree, "real-model": cmd_real_model,
            "models": cmd_models}[args.cmd](args, vocab)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except RuntimeError as exc:
        print(f"::error::{exc}", file=sys.stderr)
        sys.exit(1)
