#!/usr/bin/env python3
"""The analysis service's open issues for a pull request, as work-list items.

THE SECOND REVIEWER. SonarCloud analyses every component a pull request
touches under that component's own project, and its open issues hold the merge
exactly as a review thread does once the quality gate is required. On a
labelled pull request they join the dispatch's work list beside the threads --
collected HERE, by a program, from the service's API. The model reads no API.

ONE PROJECT PER COMPONENT, keyed exactly as `.github/actions/sonar-scan` keys
them: `<organisation>_agent-ops-operator_<component>`, the component names
read from `components.sh` (the one program that names images). Nothing here
restates that list.

"NOT YET ANALYSED FOR THIS HEAD" IS A FLAG, NEVER AN EMPTY LIST. The analysis
of a landed commit runs in `ci.yml` after the push, and a round collecting
before it finishes must not report the analysis clean because the service had
not seen the commit. Per project the status is one of:

  analysed  the pull request's latest analysis is of the head sha -> issues read
  stale     an analysis exists, of an older commit              -> not consulted
  absent    no analysis of this pull request                    -> untouched, or not yet
  error     the service could not be asked                      -> not consulted

`consulted` is true when at least one project is `analysed`; the round's
summary says so either way.

THIS PROGRAM ONLY READS. The token that lists issues could also mark them
(`won't fix`, `false positive`), and no such call exists here: a dispute is
posted on the pull request for a person, and the service's state is a
person's to change in its own UI.

The service is reached with `curl` under `SONAR_TOKEN`, so the suite can stand
in a `curl` on PATH and never touch the network.
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import subprocess
import sys
import urllib.parse

DEFAULT_API = "https://sonarcloud.io"
PAGE = 500


def fetch(api: str, path: str, token: str, **params) -> dict:
    url = f"{api}/api/{path}?{urllib.parse.urlencode(params)}"
    out = subprocess.run(["curl", "-sf", "-u", f"{token}:", url], capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError(f"{path}: curl exit {out.returncode} {out.stderr.strip()}")
    return json.loads(out.stdout or "{}")


def components(path: pathlib.Path | None, script: pathlib.Path) -> list[dict]:
    """[{component, context}] from components.sh (or a captured copy of its
    output, for the suite)."""
    if path:
        return json.loads(path.read_text())
    out = subprocess.run([str(script), "images"], capture_output=True, text=True, check=True).stdout
    return json.loads(out)


def project_status(api: str, token: str, key: str, pr: int, head: str) -> dict:
    try:
        listing = fetch(api, "project_pull_requests/list", token, project=key)
    except (RuntimeError, json.JSONDecodeError) as exc:
        return {"status": "error", "detail": str(exc)}
    for entry in listing.get("pullRequests", []):
        if str(entry.get("key")) != str(pr):
            continue
        sha = (entry.get("commit") or {}).get("sha") or ""
        gate = (entry.get("status") or {}).get("qualityGateStatus")
        if sha and head and not (sha.startswith(head) or head.startswith(sha)):
            return {"status": "stale", "analysisSha": sha, "analysisDate": entry.get("analysisDate"),
                    "qualityGate": gate}
        return {"status": "analysed", "analysisSha": sha, "analysisDate": entry.get("analysisDate"),
                "qualityGate": gate}
    return {"status": "absent"}


def issues_for(api: str, token: str, key: str, pr: int, context: str, component: str) -> list[dict]:
    items: list[dict] = []
    page = 1
    prefix = context[2:] if context.startswith("./") else context
    while True:
        payload = fetch(api, "issues/search", token, componentKeys=key, pullRequest=pr,
                        resolved="false", ps=PAGE, p=page)
        for issue in payload.get("issues", []):
            _, _, rel = (issue.get("component") or "").partition(":")
            items.append({
                "id": f"sonar:{issue['key']}",
                "source": "sonar",
                "key": issue["key"],
                "rule": issue.get("rule"),
                "severity": issue.get("severity"),
                "type": issue.get("type"),
                "component": component,
                "path": f"{prefix}/{rel}".strip("/") if rel else prefix,
                "line": issue.get("line"),
                "message": issue.get("message", ""),
            })
        total = int(payload.get("total") or 0)
        if page * PAGE >= total or not payload.get("issues"):
            return items
        page += 1


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--organization", required=True, help="the SonarCloud organisation key")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--head", default="", help="the pull request's head sha; an analysis of another commit is stale")
    ap.add_argument("--out", type=pathlib.Path, required=True)
    ap.add_argument("--repo-name", default="agent-ops-operator",
                    help="the middle of the project key, as sonar-scan/action.yml spells it")
    ap.add_argument("--components", type=pathlib.Path,
                    help="captured `components.sh images` output; default: run the script")
    ap.add_argument("--components-script", type=pathlib.Path,
                    default=pathlib.Path(__file__).resolve().parents[1] / "components.sh")
    ap.add_argument("--api", default=os.environ.get("SONAR_API", DEFAULT_API))
    args = ap.parse_args()

    token = os.environ.get("SONAR_TOKEN", "")
    if not token:
        print("SONAR_TOKEN is not set; the analysis is not consulted", file=sys.stderr)

    projects: list[dict] = []
    issues: list[dict] = []
    for c in components(args.components, args.components_script):
        key = f"{args.organization}_{args.repo_name}_{c['component']}"
        if not token:
            status = {"status": "error", "detail": "no token"}
        else:
            status = project_status(args.api, token, key, args.pr, args.head)
        entry = {"component": c["component"], "key": key, **status}
        if status["status"] == "analysed":
            try:
                found = issues_for(args.api, token, key, args.pr, c.get("context", "."), c["component"])
            except (RuntimeError, json.JSONDecodeError, KeyError) as exc:
                entry.update(status="error", detail=str(exc))
                found = []
            entry["issues"] = len(found)
            issues.extend(found)
        projects.append(entry)
        line = f"  {entry['status']:<9}{c['component']}"
        if entry["status"] == "analysed":
            line += f" — {entry['issues']} open issue(s), gate {entry.get('qualityGate') or '?'}"
        elif entry["status"] == "stale":
            line += f" — analysed at {entry.get('analysisSha', '')[:7]}, head is {args.head[:7]}"
        elif entry["status"] == "error":
            line += f" — {entry.get('detail')}"
        print(line)

    consulted = any(p["status"] == "analysed" for p in projects)
    stale = [p["component"] for p in projects if p["status"] == "stale"]
    result = {"consulted": consulted, "stale": stale, "projects": projects, "issues": issues}
    args.out.write_text(json.dumps(result, indent=2) + "\n")
    print(f"\nanalysis {'consulted' if consulted else 'NOT consulted'}: "
          f"{len(issues)} open issue(s) written to {args.out}"
          + (f"; stale for {', '.join(stale)}" if stale else ""))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as exc:
        print(f"{exc.cmd[0]} failed: {exc.stderr or exc}", file=sys.stderr)
        sys.exit(1)
