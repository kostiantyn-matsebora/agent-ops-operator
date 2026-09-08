#!/usr/bin/env python3
"""The failed required checks on a pull request's head, as work-list items.

THE THIRD REVIEWER. The review posts threads, the analysis service posts
issues, and CI posts a verdict -- and all three hold the merge. A loop that
fixed two of them while the third stayed red would end with the pull request
still blocked and nobody told why. "Approved for fixing as a whole" means a
pull request that MERGES, not one whose threads are closed.

WHICH CHECKS COUNT: the jobs `ci-green` names in its `needs:`, read from
`ci.yml` rather than restated here -- adding a required gate in this project is
a line in that list, so a copy would go stale the first time somebody added
one. A matrix job's check run carries a suffix (`images (manager)`), so the
match is on the name up to the first ` (`.

WHAT IS EXCLUDED, AND WHY:
  - the review's own jobs. The review's findings are already the work list's
    first source, collected from the threads; a failed review job is a broken
    reviewer, not a finding.
  - `ci-green` itself. It is the aggregate: it fails BECAUSE one of its needs
    failed, and reporting both would hand the fixer the symptom beside the
    cause with no way to tell them apart.
  - anything not `failure`. A `cancelled` or `timed_out` run is not a verdict
    about the tree, and `skipped` is how a job that had nothing to do reports.

NOT YET REPORTED IS A FLAG, NEVER AN EMPTY LIST -- the same rule
`sonar-issues.py` states for the analysis. A round collecting before CI has run
on the head must not report the checks clean, so `consulted` is false when no
run of the workflow has reported on this sha at all, and the summary says so.

THE LOG TAIL IS BOUNDED, and it is what a person reading the checks tab
already sees. GitHub masks registered secrets in logs, and the job that hands
this to a model holds `contents: read` -- so nothing the log could say lets
anything push.

`gh` is the only way out, so the suite stands one in on PATH and never touches
the network.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import re
import subprocess
import sys

# The review's own jobs, and the aggregate. Names, because that is what a check
# run carries.
EXCLUDED = {"ci-green"}
EXCLUDED_WORKFLOWS = {"claude-review", "review-dispatch"}


def gh(*args: str) -> str:
    out = subprocess.run(["gh", *args], capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError(f"gh {' '.join(args)}: {out.stderr.strip()}")
    return out.stdout


def required_jobs(ci: pathlib.Path) -> set[str]:
    """The jobs `ci-green` needs, from ci.yml. NEVER a copy: a required gate is
    added to this project by a line in that `needs:`, and a list restated here
    would be wrong the first time somebody added one."""
    try:
        import yaml  # noqa: PLC0415 -- optional, and the regex below is the fallback
        spec = yaml.safe_load(ci.read_text())
        needs = ((spec.get("jobs") or {}).get("ci-green") or {}).get("needs") or []
        if isinstance(needs, str):
            needs = [needs]
        if needs:
            return {str(n).strip() for n in needs}
    except (ImportError, AttributeError, TypeError, ValueError):
        pass
    m = re.search(r"^  ci-green:.*?^    needs:\s*\[(.*?)\]", ci.read_text(), re.S | re.M)
    return {n.strip() for n in m.group(1).split(",")} if m else set()


def job_name(check_name: str) -> str:
    """`images (manager)` is the `images` job. A matrix job's check run carries
    its matrix value in parentheses, and `ci-green` needs the job."""
    return check_name.split(" (", 1)[0].strip()


def check_runs(repo: str, sha: str) -> list[dict]:
    # `--method GET` IS NOT OPTIONAL. Without it `gh api` with `-f` sends the
    # params as a BODY, and this route answers a bodied GET with a plain 404 —
    # not an error a caller can tell from "no checks". gotchas.md has the day
    # that cost.
    raw = gh("api", "--method", "GET", "--paginate",
             f"repos/{repo}/commits/{sha}/check-runs", "--jq", ".check_runs[]")
    runs = []
    for line in raw.splitlines():
        line = line.strip()
        if line:
            runs.append(json.loads(line))
    return runs


def run_id(check: dict) -> str:
    """The Actions run this check run belongs to, from its details url
    (`.../actions/runs/<id>/job/<id>`)."""
    m = re.search(r"/actions/runs/(\d+)", check.get("details_url") or "")
    return m.group(1) if m else ""


def failed_log(repo: str, run: str, job: str, tail_lines: int) -> str:
    """The failed steps' log, cut to a tail. `--log-failed` prints only the
    steps that failed, which is what makes a bound of a couple of hundred lines
    enough to reproduce with."""
    if not run:
        return ""
    try:
        raw = gh("run", "view", run, "--repo", repo, "--log-failed")
    except RuntimeError as exc:
        return f"(the log could not be read: {exc})"
    lines = [ln for ln in raw.splitlines() if not job or ln.startswith(job)]
    if not lines:
        lines = raw.splitlines()
    return "\n".join(lines[-tail_lines:])


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--sha", required=True, help="the pull request's head sha")
    ap.add_argument("--out", type=pathlib.Path, required=True)
    ap.add_argument("--tail-lines", type=int, default=200,
                    help="how much of each failed job's log reaches the work list")
    ap.add_argument("--ci", type=pathlib.Path,
                    default=pathlib.Path(__file__).resolve().parents[1] / "workflows" / "ci.yml")
    args = ap.parse_args()

    required = required_jobs(args.ci)
    if not required:
        print("::warning::ci-green names no needs; no check is treated as required")

    try:
        runs = check_runs(args.repo, args.sha)
    except RuntimeError as exc:
        result = {"consulted": False, "detail": str(exc), "checks": [], "items": []}
        args.out.write_text(json.dumps(result, indent=2) + "\n")
        print(f"the checks API could not be asked: {exc}", file=sys.stderr)
        return 0

    # CONSULTED means CI reported on this sha at all — any conclusion, any of
    # its jobs. Absent, the round proceeds over the other sources and the
    # summary says the checks were not consulted, rather than implying green.
    seen = {job_name(c.get("name") or "") for c in runs}
    consulted = bool(seen & required)

    items: list[dict] = []
    checks: list[dict] = []
    for c in runs:
        name = c.get("name") or ""
        job = job_name(name)
        conclusion = c.get("conclusion")
        if job in EXCLUDED or job not in required:
            continue
        entry = {"job": name, "conclusion": conclusion}
        checks.append(entry)
        if conclusion != "failure":
            continue
        run = run_id(c)
        url = c.get("html_url") or c.get("details_url") or ""
        items.append({
            "id": f"check:{name}",
            "source": "check",
            "kind": "check",
            "job": name,
            "run_url": url,
            "path": ".github/workflows/ci.yml",
            "line": None,
            "tail": failed_log(args.repo, run, name, args.tail_lines),
        })

    result = {"consulted": consulted, "checks": checks, "items": items}
    args.out.write_text(json.dumps(result, indent=2) + "\n")
    for i in items:
        print(f"  failure  {i['job']}")
    print(f"checks {'consulted' if consulted else 'NOT consulted'}: "
          f"{len(items)} failed required check(s) written to {args.out}")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except RuntimeError as exc:
        print(f"{exc}", file=sys.stderr)
        sys.exit(1)
