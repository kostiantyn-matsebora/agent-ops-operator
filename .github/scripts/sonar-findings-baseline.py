#!/usr/bin/env python3
"""The branch-wide Blocker/High backlog, per component per rating (design D1, D3).

WHY A SECOND SCRIPT, NOT A `sonar-issues.py` MODE. That script's whole contract
is "one pull request's issues, for the autofix loop" -- keyed on `--pr` and
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
import re
import subprocess
import sys
import urllib.parse

DEFAULT_API = "https://sonarcloud.io"
PAGE = 500
QUALITIES = ("RELIABILITY", "SECURITY", "MAINTAINABILITY")


def validated_api(raw: str) -> str:
    """Refuses anything but an http(s) URL -- the base every request below is
    built from, and the one CLI-supplied string this script must not hand to
    `curl` unexamined (pythonsecurity:S8701/S8705)."""
    parsed = urllib.parse.urlparse(raw)
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        raise SystemExit(f"--api must be an http(s) URL, not {raw!r}")
    return raw


def validated_path(raw: pathlib.Path, *, must_exist: bool) -> pathlib.Path:
    """Canonicalises the path and refuses one that resolves outside the
    current working directory -- the exact pythonsecurity:S2083/S8707
    remediation (their own compliant example: `os.path.realpath` against
    `os.getcwd()`, checked with the trailing separator the rule's own
    "partial path traversal" pitfall warns is required), applied to every
    CLI-supplied path: `--out`, `--components` and `--components-script`.
    """
    base_dir = os.path.realpath(os.getcwd())
    resolved = os.path.realpath(str(raw))
    if resolved != base_dir and not resolved.startswith(base_dir + os.sep):
        raise SystemExit(f"path resolves outside the working directory: {raw}")
    result = pathlib.Path(resolved)
    if must_exist and not result.is_file():
        raise SystemExit(f"not a file: {result}")
    return result


SAFE_URL = re.compile(r"^https?://[\w.\-~:/]+\?[\w.\-~%=&]*$")


def fetch(api: str, path: str, token: str, **params) -> dict:
    url = f"{api}/api/{path}?{urllib.parse.urlencode(params)}"
    # Validated immediately before the subprocess call it guards, the same
    # shape as pythonsecurity:S8705's own compliant example (a regex check
    # adjacent to the sink) -- a validation several statements away was not
    # credited, same lesson as `validated_path` below. `--` marks the end
    # of options too: verified against curl itself that without it a URL
    # beginning with "-" is read as an unrecognised flag ("curl: option
    # ...: is unknown", exit 2) rather than the target -- the concrete case
    # this regex also rules out, since only an http(s) scheme passes it.
    if not SAFE_URL.match(url):
        raise SystemExit(f"refusing a malformed request URL: {url!r}")
    out = subprocess.run(["curl", "-sf", "-u", f"{token}:", "--", url], capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError(f"{path}: curl exit {out.returncode} {out.stderr.strip()}")
    return json.loads(out.stdout or "{}")


def components(path: pathlib.Path | None, script: pathlib.Path) -> list[dict]:
    """[{component, ...}] from components.sh (or a captured copy, for the suite)."""
    if path:
        return json.loads(validated_path(path, must_exist=True).read_text())
    resolved = validated_path(script, must_exist=True)
    if not os.access(resolved, os.X_OK):
        raise SystemExit(f"not executable: {resolved}")
    out = subprocess.run([str(resolved), "images"], capture_output=True, text=True, check=True).stdout
    return json.loads(out)


def issues_for(api: str, token: str, key: str, **filters) -> list[dict]:
    """Every open (`resolved=false`) issue on `key` matching `filters`, paged.
    NO `pullRequest` param -- the branch-wide backlog, not one pull request's."""
    items: list[dict] = []
    page = 1
    while True:
        payload = fetch(api, "issues/search", token, componentKeys=key, resolved="false",
                         ps=PAGE, p=page, **filters)
        found = payload.get("issues", [])
        items.extend(found)
        total = int(payload.get("total") or 0)
        if page * PAGE >= total or not found:
            return items
        page += 1


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
    entry: dict = {"key": key, "counts": counts, "total": len(found)}
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
    # Fails fast, before any network call -- but the WRITE below revalidates
    # inline rather than trusting this one across the whole function: kept
    # directly adjacent to the sink it guards, the same shape as the rule's
    # own `open(safe_path(filename))` example, since a validation several
    # statements and a loop away from its `.write_text()` was not credited.
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
            entry = {"key": key, "counts": dict.fromkeys(QUALITIES, 0), "total": 0,
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

    result = {"organization": args.organization, "components": rows}
    # ONE EXPRESSION, not a validated variable carried to a later
    # statement: the shape components()'s (never-flagged) read side already
    # uses, and pythonsecurity:S2083/S8707's own `open(safe_path(filename))`
    # example -- a sanitiser call the line above the sink it guards was
    # still flagged, so the call itself is now the sink's own argument.
    validated_path(args.out, must_exist=False).write_text(json.dumps(result, indent=2) + "\n")
    total = sum(r["total"] for r in rows)
    print(f"\n{total} open Blocker/High finding(s) across {len(rows)} project(s), written to {validated_path(args.out, must_exist=False)}"
          + (f"; TAXONOMY MISMATCH for {', '.join(mismatches)}" if mismatches else ""))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as exc:
        print(f"{exc.cmd[0]} failed: {exc.stderr or exc}", file=sys.stderr)
        sys.exit(1)
