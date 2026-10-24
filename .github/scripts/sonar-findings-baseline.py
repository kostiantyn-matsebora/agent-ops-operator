#!/usr/bin/env python3
"""The branch-wide Blocker/High backlog, per component per rating (design D1, D3).

WHY A SECOND SCRIPT, NOT A `sonar-issues.py` MODE. That script's whole contract
is "one pull request's issues, for the fixing loop" -- keyed on `--pr` and
`--head`, and every caller today is a workflow that always passes both. This
reads a component's BRANCH-WIDE backlog instead: no `pullRequest` filter, run
once by hand, never by CI.

THE ORGANISATION IS ON THE CLEAN CODE TAXONOMY (MQR MODE): an issue carries
`impacts[]`, each `{softwareQuality, severity}`, filtered here with
`impactSeverities=BLOCKER,HIGH`. The RETIRED five-level `severity` field
(BLOCKER/CRITICAL/MAJOR/MINOR/INFO) is the legacy taxonomy this is not -- a
0-result Clean Code query is indistinguishable from a genuinely clean backlog
and from an organisation still on the legacy scale, so a component with
nothing found is re-asked with `severities=BLOCKER,CRITICAL` and BOTH counts
are reported when they disagree, rather than trusting the new one silently.

Counts per component per `softwareQuality` (reliability, security,
maintainability -- the three overall ratings `sonar-provision.sh --gate`
conditions on) are written to a JSON file and printed as a table. No finding
text and no organisation identifier reach the table this feeds
(`tasks.md` 1.2) -- `publication.md`'s rule for a secret and for a message a
person wrote, same as `coverage-across-packages`' task 1.1 established.

Reached with `curl` under `SONAR_TOKEN`, so the suite can stand in a `curl` on
PATH and never touch the network.
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from sonar_api import DEFAULT_API, components, issues_for, validated_api, validated_path  # noqa: E402

QUALITIES = ("RELIABILITY", "SECURITY", "MAINTAINABILITY")


def write_result(out: pathlib.Path, result: dict) -> pathlib.Path:
    """Validates `out` and writes `result` to it, returning the path used.

    A DEDICATED FUNCTION, not inlined at the call site in `main` -- the
    working reference for pythonsecurity:S2083/S8707 is `sonar_api.
    components`, which takes its CLI-supplied path as its OWN parameter and
    validates it there; three earlier attempts at `main`'s own scope
    (a separated variable, adjacent statements, one nested expression)
    all stayed flagged on `args.out` specifically. This crosses the same
    function boundary `components` does rather than guessing at another
    same-scope shape."""
    target = validated_path(out, must_exist=False)
    target.write_text(json.dumps(result, indent=2) + "\n")
    return target


def clean_code_counts(found: list[dict]) -> dict[str, int]:
    """Per-quality counts of ISSUES (not impacts) carrying a Blocker/High
    impact on that quality -- what each overall rating condition is judged on."""
    counts = dict.fromkeys(QUALITIES, 0)
    for issue in found:
        for impact in issue.get("impacts", []):
            if impact.get("severity") in ("BLOCKER", "HIGH") and impact.get("softwareQuality") in counts:
                counts[impact["softwareQuality"]] += 1
    return counts


def baseline_for(api: str, token: str, key: str) -> dict:
    found = issues_for(api, token, key, impactSeverities="BLOCKER,HIGH")
    counts = clean_code_counts(found)
    # `total` is the FINDING count (one issue may carry impacts on more than one
    # quality, so it can be less than the sum of `counts`); the sum is what a
    # zero-result taxonomy check below is keyed on, since the two agree exactly
    # when nothing was found at all.
    #
    # NO "key" HERE: it is `{organization}_{repo_name}_{component}`, and the
    # docstring's guarantee is that no organisation identifier reaches this
    # file. The caller already records the bare component name separately.
    entry: dict = {"counts": counts, "total": len(found)}
    if sum(counts.values()) == 0:
        legacy = issues_for(api, token, key, severities="BLOCKER,CRITICAL")
        entry["legacyCount"] = len(legacy)
        entry["taxonomyMismatch"] = len(legacy) > 0
    else:
        entry["legacyCount"] = None
        entry["taxonomyMismatch"] = False
    return entry


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--organization", required=True, help="the SonarCloud organisation key")
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
        print("SONAR_TOKEN is not set; the organisation is not consulted", file=sys.stderr)
        return 1

    args.api = validated_api(args.api)
    # Fails fast, before any network call -- write_result (below) validates
    # again itself, inside its own scope, when it actually writes.
    validated_path(args.out, must_exist=False)

    # THE SAME LIST sonar-provision.sh PROVISIONS: every component plus
    # `scripts`, which is a project and not a component -- see that script's
    # stage 1 comment for why it is appended here rather than taught to
    # components.sh.
    names = [c["component"] for c in components(args.components, args.components_script)] + ["scripts"]

    rows: list[dict] = []
    mismatches: list[str] = []
    for name in names:
        key = f"{args.organization}_{args.repo_name}_{name}"
        try:
            entry = baseline_for(args.api, token, key)
        except (RuntimeError, json.JSONDecodeError) as exc:
            entry = {"counts": dict.fromkeys(QUALITIES, 0), "total": 0,
                      "legacyCount": None, "taxonomyMismatch": False, "error": str(exc)}
        entry["component"] = name
        rows.append(entry)
        line = f"  {name:<20}" + "  ".join(f"{q[:4].title()}={entry['counts'][q]}" for q in QUALITIES)
        if entry.get("error"):
            line += f"  ERROR: {entry['error']}"
        elif entry["taxonomyMismatch"]:
            line += (f"  TAXONOMY MISMATCH: 0 Clean Code, {entry['legacyCount']} "
                     "legacy-severity issue(s) -- provisioning/enumeration may be reading the wrong scale")
            mismatches.append(name)
        print(line)

    # No "organization" key: the docstring's guarantee is that no
    # organisation identifier reaches this file, and args.organization
    # embedded here would have been exactly that.
    result = {"components": rows}
    out_path = write_result(args.out, result)
    total = sum(r["total"] for r in rows)
    print(f"\n{total} open Blocker/High finding(s) across {len(rows)} project(s), written to {out_path}"
          + (f"; TAXONOMY MISMATCH for {', '.join(mismatches)}" if mismatches else ""))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as exc:
        print(f"{exc.cmd[0]} failed: {exc.stderr or exc}", file=sys.stderr)
        sys.exit(1)
