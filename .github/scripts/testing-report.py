#!/usr/bin/env python3
"""The testing station's verdict and its one report on the tracking issue.

  judge   --tier-file unit=F ... [--outcome unit=passed|failed|skipped ...]
          coverage-diff.py's output per tier plus whether the tier's TESTS passed,
          to a verdict per tier and the round's work list (conveyor.judge_tier).
  render  --verdict F --round N --cap M [--state running|passed|capped]
          the report as markdown. Paths and line numbers only, never a line of source.
  post    --issue N --body-file F
          create the one comment carrying the marker, or edit it in place.

THE FLOORS ARE THE VOCABULARY'S (`testing.floors`), not constants here, so the number a
report prints and the number a decision judged against cannot differ.

ONE COMMENT, EDITED IN PLACE. A comment per round is five comments on an issue a person
reads for the state of the line. The marker is a hidden line the script finds it by.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402

VOCAB = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
MAX_FILES = 15
MAX_RANGES = 12

ICON = {"pass": "pass", "under_floor": "under floor", "tests_failed": "tests failed",
        "collection_failed": "no coverage data", "not_applicable": "not applicable", "not_run": "not run"}


def tier_result(cov: dict | None, outcome: str) -> "conveyor.TierResult":
    """A coverage-diff output and the tier's test outcome as the machine's fact.

    A tier whose domain holds no changed file is `not applicable` whatever else it did,
    so its tests alone can still fail it but its coverage never can."""
    domain_empty = cov is not None and cov.get("domain_files", 0) == 0
    # A TIER WITH NOTHING TO MEASURE IS `not applicable`, never `not run`: skipping a tier the change cannot reach
    # is no gap. Only a tier that has changed lines in its domain and did not run is.
    ran = outcome != "skipped" or domain_empty
    passed = outcome != "failed"
    if cov is None:
        return conveyor.TierResult(ran=ran, passed=passed, collected=False)
    if cov.get("domain_files", 0) == 0:
        # Checked BEFORE `ran`: a skipped tier with nothing to measure is not applicable, never not run.
        return conveyor.TierResult(ran=True, passed=passed, covered=0, total=0, collected=True)
    return conveyor.TierResult(ran=ran, passed=passed, covered=cov["covered"], total=cov["total"],
                               collected=bool(cov.get("collected")))


def judge(vocab: dict, covs: dict, outcomes: dict, partial=()) -> dict:
    """Every tier needs an explicit outcome, and only a known tier may be named.

    A MISSING OUTCOME IS NOT `passed`: defaulting it let a tier nobody ran read as a run that left no
    coverage (`collection_failed`) instead of `not_run`, and a misspelt tier name vanished without a word."""
    known = set(conveyor.TIERS)
    for name in list(covs) + list(outcomes):
        if name not in known:
            raise ValueError(f"unknown tier {name!r}: the tiers are {', '.join(conveyor.TIERS)}")
    missing = [t for t in conveyor.TIERS if t not in outcomes]
    if missing:
        raise ValueError("no --outcome for " + ", ".join(missing) + ": give passed, failed or skipped for every tier")
    # A TIER IS PASSED ONLY ON A FULL RUN. A tier whose numbers come from a partial re-run reads as not run, so it
    # cannot end the station: a fix can break a test the re-run never touched.
    for name in partial:
        if name not in known:
            raise ValueError(f"unknown tier {name!r} in --partial")
        if outcomes.get(name) == "passed":
            outcomes = {**outcomes, name: "skipped"}
    floors = vocab["testing"]["floors"]
    verdicts = []
    for tier in conveyor.TIERS:
        r = tier_result(covs.get(tier), outcomes[tier])
        verdicts.append(conveyor.judge_tier(tier, floors[tier], r))
    work = conveyor.testing_work(verdicts)
    details = {}
    for tier in conveyor.TIERS:
        cov = covs.get(tier) or {}
        details[tier] = {"covered": cov.get("covered", 0), "total": cov.get("total", 0),
                         "files": cov.get("files", {}), "unmeasured": cov.get("unmeasured", [])}
    from dataclasses import asdict
    return {"verdicts": [asdict(v) for v in verdicts], "work": [asdict(w) for w in work], "details": details}


def ranges(lines) -> str:
    """[1,2,3,7,9,10] -> '1-3, 7, 9-10'."""
    out, start, prev = [], None, None
    for ln in sorted(lines):
        if start is None:
            start = prev = ln
        elif ln == prev + 1:
            prev = ln
        else:
            out.append((start, prev)); start = prev = ln
    if start is not None:
        out.append((start, prev))
    shown = [f"{a}" if a == b else f"{a}-{b}" for a, b in out[:MAX_RANGES]]
    more = len(out) - MAX_RANGES
    return ", ".join(shown) + (f", +{more} more" if more > 0 else "")


def render(vocab: dict, verdict: dict, number: int, cap: int, state: str = "running") -> str:
    t = vocab["testing"]
    rows = ["| Tier | Floor | Changed lines covered | Result |", "|---|---|---|---|"]
    for v in verdict["verdicts"]:
        d = verdict["details"][v["tier"]]
        pct = "n/a" if v["percent"] is None else f"{v['percent']}%"
        counts = "" if v["percent"] is None else f" ({d['covered']} of {d['total']})"
        floor = f"{v['floor']}%" if v["status"] != "not_applicable" else "none"
        rows.append(f"| {v['tier']} | {floor} | {pct}{counts} | {ICON[v['status']]} |")
    verdict_line = {"passed": "Every tier passes and holds its floor.",
                    "capped": "The testing budget is spent with items left. A person decides.",
                    "running": f"{len(verdict['work'])} item(s) left. The tiers run again."}[state]
    out = [t["coverage_marker"], "### Testing station", "",
           f"Round {number} of {cap}. Coverage is measured on the lines this change added.", "", *rows, ""]
    left = []
    for tier in conveyor.TIERS:
        d = verdict["details"][tier]
        for path, lines in sorted(d["files"].items())[:MAX_FILES]:
            left.append(f"- `{path}` ({tier}): lines {ranges(lines)}")
        extra = len(d["files"]) - MAX_FILES
        if extra > 0:
            left.append(f"- {extra} more file(s) under {tier}")
    if left:
        out += ["Uncovered changed lines:", "", *left, ""]
    unmeasured = []
    for tier in conveyor.TIERS:
        for path in verdict["details"][tier].get("unmeasured", [])[:MAX_FILES]:
            unmeasured.append(f"- `{path}` ({tier})")
    if unmeasured:
        out += ["Changed files with no coverage data (every added line counts as uncovered):", "", *unmeasured, ""]
    if verdict["work"]:
        w = ", ".join(sorted({f"{x['kind'].replace('_', ' ')} ({x['tier']}, {x['session']})" for x in verdict["work"]}))
        out += [f"Work: {w}.", ""]
    out += [f"**{verdict_line}**"]
    return "\n".join(out) + "\n"


def _gh(*args, stdin=None):
    return subprocess.run(["gh", *args], input=stdin, capture_output=True, text=True, check=True).stdout


def post(vocab: dict, issue: int, body: str, repo: str = "") -> str:
    marker = vocab["testing"]["coverage_marker"]
    repo = repo or _gh("repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner").strip()
    comments = json.loads(_gh("api", "--paginate", "--slurp", f"repos/{repo}/issues/{issue}/comments"))
    flat = [c for page in comments for c in (page if isinstance(page, list) else [page])]
    mine = [c for c in flat if marker in (c.get("body") or "")]
    if mine:
        cid = mine[-1]["id"]
        _gh("api", "--method", "PATCH", f"repos/{repo}/issues/comments/{cid}", "--input", "-",
            stdin=json.dumps({"body": body}))
        return f"edited comment {cid}"
    _gh("api", "--method", "POST", f"repos/{repo}/issues/{issue}/comments", "--input", "-",
        stdin=json.dumps({"body": body}))
    return "created the comment"


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--vocabulary", type=pathlib.Path, default=VOCAB)
    sub = ap.add_subparsers(dest="cmd", required=True)
    j = sub.add_parser("judge")
    j.add_argument("--tier-file", action="append", default=[], metavar="TIER=FILE")
    j.add_argument("--outcome", action="append", default=[], metavar="TIER=passed|failed|skipped")
    r = sub.add_parser("render")
    r.add_argument("--verdict", required=True)
    r.add_argument("--round", type=int, required=True)
    r.add_argument("--cap", type=int, required=True)
    r.add_argument("--state", default="running", choices=["running", "passed", "capped"])
    p = sub.add_parser("post")
    p.add_argument("--issue", type=int, required=True)
    p.add_argument("--body-file", required=True)
    p.add_argument("--repo", default="")
    args = ap.parse_args(argv)
    vocab = json.loads(args.vocabulary.read_text())

    if args.cmd == "judge":
        covs = {}
        for kv in args.tier_file:
            tier, _, path = kv.partition("=")
            f = pathlib.Path(path)
            covs[tier] = json.loads(f.read_text()) if f.exists() and f.stat().st_size else None
        outcomes = dict(kv.split("=", 1) for kv in args.outcome)
        json.dump(judge(vocab, covs, outcomes), sys.stdout, indent=2, sort_keys=True)
        print()
    elif args.cmd == "render":
        sys.stdout.write(render(vocab, json.loads(pathlib.Path(args.verdict).read_text()), args.round, args.cap, args.state))
    else:
        print(post(vocab, args.issue, pathlib.Path(args.body_file).read_text(), args.repo))
    return 0


if __name__ == "__main__":
    sys.exit(main())
